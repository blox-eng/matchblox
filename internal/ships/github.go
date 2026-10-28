package ships

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GHRunner runs the GitHub CLI and returns what it wrote to stdout.
type GHRunner func(ctx context.Context, args ...string) ([]byte, error)

var errNoGH = errors.New("gh not installed")

// GH runs the GitHub CLI. Its error carries what gh wrote to stderr.
func GH(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errNoGH
	}
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return out, err
}

// why says in a few words why GitHub could not be read.
func why(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, errNoGH):
		return "gh not installed"
	case strings.Contains(msg, "auth login"), strings.Contains(msg, "not logged"), strings.Contains(msg, "bad credentials"), strings.Contains(msg, "401"):
		return "gh not signed in"
	case strings.Contains(msg, "rate limit"):
		return "GitHub rate limit"
	}
	if i := strings.IndexByte(msg, '\n'); i > 0 {
		msg = msg[:i]
	}
	if len(msg) > 60 {
		msg = msg[:60]
	}
	return msg
}

// facts are what GitHub said about a pull request at its last read.
type facts struct {
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	State      string    `json:"state"` // OPEN | MERGED | CLOSED
	Draft      bool      `json:"draft,omitempty"`
	Mergeable  string    `json:"mergeable,omitempty"`   // MERGEABLE | CONFLICTING | UNKNOWN
	MergeState string    `json:"merge_state,omitempty"` // CLEAN | BEHIND | BLOCKED | DIRTY | ...
	Review     string    `json:"review,omitempty"`      // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED
	AutoMerge  bool      `json:"auto_merge,omitempty"`
	Base       string    `json:"base"`
	Head       string    `json:"head"`
	Fork       bool      `json:"fork,omitempty"`     // its head is a branch of another repository
	HeadOID    string    `json:"head_oid,omitempty"` // the head commit: a merge is pinned to it
	Created    time.Time `json:"created"`
	Merged     time.Time `json:"merged,omitzero"`
	MergeOID   string    `json:"merge_oid,omitempty"`
	Checks     checks    `json:"checks"`
	Runs       []run     `json:"runs,omitempty"` // the workflow runs on the merge commit
	Method     string    `json:"method"`         // the repository's merge: squash, merge or rebase
	Fetched    time.Time `json:"fetched"`
}

// checks count the check runs and statuses of the head commit.
type checks struct {
	Total      int       `json:"total"`
	Completed  int       `json:"completed"`
	Failed     int       `json:"failed"`
	FailedName string    `json:"failed_name,omitempty"`
	Started    time.Time `json:"started,omitzero"`
	Done       time.Time `json:"done,omitzero"` // the last one finished
}

type run struct {
	Workflow   string    `json:"workflow"`
	Status     string    `json:"status"`               // QUEUED | IN_PROGRESS | COMPLETED | ...
	Conclusion string    `json:"conclusion,omitempty"` // SUCCESS | FAILURE | CANCELLED | SKIPPED | ...
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated"`
	// Later is a newer run of the same workflow on the default branch, for
	// a failed, cancelled or skipped one: success, running, empty for none,
	// or ? before the default branch was read.
	Later string `json:"later,omitempty"`
}

// unfinished: a completed run that did not pass, so a newer run of its
// workflow on the default branch decides whether it counts.
func (r run) unfinished() bool {
	return r.Status == "COMPLETED" && r.Conclusion != "SUCCESS" && r.Conclusion != "NEUTRAL" && r.Conclusion != ""
}

// superseded: a run of the merge commit waits for a newer run on the
// default branch to tell whether it counts.
func (f *facts) superseded() bool {
	for _, r := range f.Runs {
		if r.unfinished() && r.Later != "success" {
			return true
		}
	}
	return false
}

// query is the one GraphQL query of a repository's poll: its pull requests
// in flight, the default branch's recent runs when a merged one needs
// them, and its open pull requests when a push waits for its PR.
func query(owner, name string, numbers []int, history, open bool) string {
	var b strings.Builder
	b.WriteString("query{rateLimit{limit remaining resetAt}")
	fmt.Fprintf(&b, "repository(owner:%q,name:%q){squashMergeAllowed mergeCommitAllowed rebaseMergeAllowed", owner, name)
	if history {
		b.WriteString(" defaultBranchRef{target{...on Commit{history(first:10){nodes{checkSuites(first:20){nodes{...suite}}}}}}}")
	}
	if open {
		b.WriteString(" open:pullRequests(states:OPEN,first:100,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number headRefName isCrossRepository}}")
	}
	for _, n := range numbers {
		fmt.Fprintf(&b, " pr%d:pullRequest(number:%d){...pr}", n, n)
	}
	b.WriteString("}}")
	b.WriteString(`fragment suite on CheckSuite{status conclusion updatedAt workflowRun{createdAt workflow{name}}}`)
	b.WriteString(`fragment pr on PullRequest{number title url state isDraft mergeable mergeStateStatus reviewDecision ` +
		`baseRefName headRefName isCrossRepository headRefOid createdAt mergedAt autoMergeRequest{enabledAt} ` +
		`mergeCommit{oid checkSuites(first:30){nodes{...suite}}} ` +
		`commits(last:1){nodes{commit{statusCheckRollup{contexts(first:100){` +
		`checkRunCount checkRunCountsByState{state count} statusContextCount statusContextCountsByState{state count} ` +
		`nodes{__typename ...on CheckRun{name status conclusion startedAt completedAt} ...on StatusContext{context state createdAt}}}}}}}}`)
	return b.String()
}

type suiteJSON struct {
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	UpdatedAt   time.Time `json:"updatedAt"`
	WorkflowRun *struct {
		CreatedAt time.Time `json:"createdAt"`
		Workflow  struct {
			Name string `json:"name"`
		} `json:"workflow"`
	} `json:"workflowRun"`
}

type suites struct {
	Nodes []suiteJSON `json:"nodes"`
}

type countJSON struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type prJSON struct {
	Number           int       `json:"number"`
	Title            string    `json:"title"`
	URL              string    `json:"url"`
	State            string    `json:"state"`
	IsDraft          bool      `json:"isDraft"`
	Mergeable        string    `json:"mergeable"`
	MergeStateStatus string    `json:"mergeStateStatus"`
	ReviewDecision   string    `json:"reviewDecision"`
	BaseRefName      string    `json:"baseRefName"`
	HeadRefName      string    `json:"headRefName"`
	Fork             bool      `json:"isCrossRepository"`
	HeadRefOid       string    `json:"headRefOid"`
	CreatedAt        time.Time `json:"createdAt"`
	MergedAt         time.Time `json:"mergedAt"`
	AutoMergeRequest *struct{} `json:"autoMergeRequest"`
	MergeCommit      *struct {
		OID         string `json:"oid"`
		CheckSuites suites `json:"checkSuites"`
	} `json:"mergeCommit"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						CheckRunCount              int         `json:"checkRunCount"`
						CheckRunCountsByState      []countJSON `json:"checkRunCountsByState"`
						StatusContextCount         int         `json:"statusContextCount"`
						StatusContextCountsByState []countJSON `json:"statusContextCountsByState"`
						Nodes                      []struct {
							Typename    string    `json:"__typename"`
							Name        string    `json:"name"`
							Status      string    `json:"status"`
							Conclusion  string    `json:"conclusion"`
							StartedAt   time.Time `json:"startedAt"`
							CompletedAt time.Time `json:"completedAt"`
							Context     string    `json:"context"`
							State       string    `json:"state"`
							CreatedAt   time.Time `json:"createdAt"`
						} `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type rateJSON struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"resetAt"`
}

// answer is one repository's poll, read.
type answer struct {
	Rate *rateJSON
	// PRs maps the pull requests read; a nil value is one GitHub says does
	// not exist. One it could not read (forbidden, a timeout) is absent:
	// it keeps what was read before.
	PRs   map[int]*facts
	Heads map[string]int // open PRs by head branch, when asked for
}

// badPRs is a query GitHub refused whole because of some of its pull
// requests (a number it cannot take): the rest can be asked without them.
type badPRs struct {
	numbers []int
	msg     string
}

func (e badPRs) Error() string { return e.msg }

type gqlError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Path    []any  `json:"path"`
}

// prOf is the pull request an error's path names ("pr123"), or 0.
func (e gqlError) prOf() int {
	for _, p := range e.Path {
		if s, ok := p.(string); ok && strings.HasPrefix(s, "pr") {
			if n, err := strconv.Atoi(s[2:]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// parseAnswer reads a GraphQL response.
func parseAnswer(b []byte, numbers []int, history bool, now time.Time) (answer, error) {
	var resp struct {
		Data *struct {
			RateLimit  *rateJSON                  `json:"rateLimit"`
			Repository map[string]json.RawMessage `json:"repository"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return answer{}, err
	}
	if resp.Data == nil {
		var bad []int
		for _, e := range resp.Errors {
			if n := e.prOf(); n > 0 && !slices.Contains(bad, n) {
				bad = append(bad, n)
			}
		}
		switch {
		case len(bad) > 0:
			return answer{}, badPRs{numbers: bad, msg: resp.Errors[0].Message}
		case len(resp.Errors) > 0:
			return answer{}, errors.New(resp.Errors[0].Message)
		}
		return answer{}, errors.New("no data")
	}
	gone := map[int]bool{}
	for _, e := range resp.Errors {
		if n := e.prOf(); n > 0 && e.Type == "NOT_FOUND" {
			gone[n] = true
		}
	}
	a := answer{Rate: resp.Data.RateLimit, PRs: map[int]*facts{}}
	repo := resp.Data.Repository
	if repo == nil {
		if len(resp.Errors) > 0 {
			return a, errors.New(resp.Errors[0].Message)
		}
		return a, errors.New("no repository")
	}
	method := "squash"
	switch allowed := func(k string) bool { return string(repo[k]) == "true" }; {
	case allowed("squashMergeAllowed"):
	case allowed("mergeCommitAllowed"):
		method = "merge"
	case allowed("rebaseMergeAllowed"):
		method = "rebase"
	}
	var runs []suiteJSON
	if raw, ok := repo["defaultBranchRef"]; ok {
		var ref struct {
			Target struct {
				History struct {
					Nodes []struct {
						CheckSuites suites `json:"checkSuites"`
					} `json:"nodes"`
				} `json:"history"`
			} `json:"target"`
		}
		if json.Unmarshal(raw, &ref) == nil {
			for _, c := range ref.Target.History.Nodes {
				runs = append(runs, c.CheckSuites.Nodes...)
			}
		}
	}
	if raw, ok := repo["open"]; ok {
		var open struct {
			Nodes []struct {
				Number int    `json:"number"`
				Head   string `json:"headRefName"`
				Fork   bool   `json:"isCrossRepository"`
			} `json:"nodes"`
		}
		if json.Unmarshal(raw, &open) == nil {
			a.Heads = map[string]int{}
			for _, p := range open.Nodes {
				if !p.Fork {
					a.Heads[p.Head] = p.Number
				}
			}
		}
	}
	for _, n := range numbers {
		raw, ok := repo["pr"+strconv.Itoa(n)]
		if !ok || string(raw) == "null" {
			if gone[n] {
				a.PRs[n] = nil
			}
			continue
		}
		var p prJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			return a, err
		}
		f := factsOf(p, runs, history, now)
		f.Method = method
		a.PRs[n] = &f
	}
	return a, nil
}

// failed are the check states that fail a rollup.
var failed = set("FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED", "CANCELLED", "ERROR")

// waiting are the check states of a check that has not finished.
var waiting = set("QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "REQUESTED", "EXPECTED")

// factsOf reads one pull request. history: the default branch's recent
// runs were asked for; without them a cancelled or skipped run's Later is
// "?", and the next poll asks.
func factsOf(p prJSON, history []suiteJSON, asked bool, now time.Time) facts {
	f := facts{
		Title: clean(p.Title), URL: p.URL, State: p.State, Draft: p.IsDraft,
		Mergeable: p.Mergeable, MergeState: p.MergeStateStatus, Review: p.ReviewDecision,
		AutoMerge: p.AutoMergeRequest != nil, Base: p.BaseRefName, Head: p.HeadRefName, Fork: p.Fork, HeadOID: p.HeadRefOid,
		Created: p.CreatedAt, Merged: p.MergedAt, Fetched: now,
	}
	if n := p.Commits.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
		ctx := n[0].Commit.StatusCheckRollup.Contexts
		f.Checks.Total = ctx.CheckRunCount + ctx.StatusContextCount
		for _, c := range append(append([]countJSON{}, ctx.CheckRunCountsByState...), ctx.StatusContextCountsByState...) {
			if !waiting[c.State] {
				f.Checks.Completed += c.Count
			}
			if failed[c.State] {
				f.Checks.Failed += c.Count
			}
		}
		for _, c := range ctx.Nodes {
			name, state, started, done := c.Name, c.Conclusion, c.StartedAt, c.CompletedAt
			if c.Typename == "StatusContext" {
				name, state, started, done = c.Context, c.State, c.CreatedAt, time.Time{}
				if !waiting[state] {
					done = c.CreatedAt
				}
			}
			if failed[state] && f.Checks.FailedName == "" {
				f.Checks.FailedName = name
			}
			if !started.IsZero() && (f.Checks.Started.IsZero() || started.Before(f.Checks.Started)) {
				f.Checks.Started = started
			}
			if done.After(f.Checks.Done) {
				f.Checks.Done = done
			}
		}
	}
	if mc := p.MergeCommit; mc != nil && p.State == "MERGED" {
		f.MergeOID = mc.OID
		for _, s := range mc.CheckSuites.Nodes {
			if s.WorkflowRun == nil {
				continue // an app's suite with no workflow run in it
			}
			r := run{Workflow: s.WorkflowRun.Workflow.Name, Status: s.Status, Conclusion: s.Conclusion, Created: s.WorkflowRun.CreatedAt, Updated: s.UpdatedAt}
			switch {
			case !r.unfinished():
			case asked:
				r.Later = later(r, history)
			default:
				r.Later = "?"
			}
			f.Runs = append(f.Runs, r)
		}
		sort.Slice(f.Runs, func(i, j int) bool { return f.Runs[i].Created.Before(f.Runs[j].Created) })
	}
	return f
}

// later reads the default branch's recent runs for a newer run of r's
// workflow: success when one succeeded, running when one still runs.
func later(r run, history []suiteJSON) string {
	out := ""
	for _, s := range history {
		if s.WorkflowRun == nil || s.WorkflowRun.Workflow.Name != r.Workflow || !s.WorkflowRun.CreatedAt.After(r.Created) {
			continue
		}
		switch {
		case s.Conclusion == "SUCCESS":
			return "success"
		case s.Status != "COMPLETED":
			out = "running"
		}
	}
	return out
}

// clean drops control characters: a title is not trusted.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}
