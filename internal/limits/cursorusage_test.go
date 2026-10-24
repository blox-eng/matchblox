package limits

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cursorUsage is GetCurrentPeriodUsage in the shape Cursor's API answers it,
// with synthetic figures.
const cursorUsage = `{"billingCycleStart":"1790000000000","billingCycleEnd":"1792678400000",` +
	`"planUsage":{"totalSpend":500,"includedSpend":500,"remaining":1500,"limit":2000,"autoPercentUsed":12.5,"apiPercentUsed":2,"totalPercentUsed":25},` +
	`"spendLimitUsage":{"totalSpend":1000,"pooledLimit":"100000","pooledUsed":1000,"limitType":"team"},"enabled":true}`

// TestCursorUsageIsItsMonth: Cursor's GetCurrentPeriodUsage, in the shape
// Cursor's API answers it (synthetic figures), is the month of the plan: the
// share of the included usage used, until the billing cycle ends.
func TestCursorUsageIsItsMonth(t *testing.T) {
	now := time.UnixMilli(1791000000000)
	r, err := ParseCursorUsage([]byte(cursorUsage), now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.At.Equal(now) || len(r.Windows) != 1 {
		t.Fatalf("reading %+v", r)
	}
	w := r.Windows[0]
	start, end := time.UnixMilli(1790000000000), time.UnixMilli(1792678400000)
	if w.UsedPct != 25 || !w.ResetsAt.Equal(end) || w.Minutes != int(end.Sub(start).Minutes()) {
		t.Errorf("window %+v, want 25%% until %v", w, end)
	}
	a := Build("cursor", "you@example.com · Acme", SourceCursor, r, now, Quiet{})
	if u := a.Uses(now); a.State != Measured || len(u) != 1 || u[0].Name != "mo" || u[0].Pct != 25 {
		t.Errorf("account %+v uses %+v, want one measured mo of 25%%", a, u)
	}
}

func TestCursorUsageWithoutAPlanIsNoReading(t *testing.T) {
	if _, err := ParseCursorUsage([]byte(`{"enabled":false}`), time.Now()); err == nil {
		t.Error("no planUsage parsed, want an error")
	}
}

// TestCursorUsageAsksCursorsAPI: the call is the one Cursor's CLI makes, its
// token in the Authorization header only.
func TestCursorUsageAsksCursorsAPI(t *testing.T) {
	var got *http.Request
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		if status != http.StatusOK {
			http.Error(w, `{"code":"unauthenticated"}`, status)
			return
		}
		_, _ = w.Write([]byte(cursorUsage))
	}))
	defer srv.Close()
	b, err := FetchCursorUsage(context.Background(), srv.Client(), srv.URL, "tok")
	if err != nil || len(b) == 0 {
		t.Fatalf("fetch: %v %q", err, b)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/aiserver.v1.DashboardService/GetCurrentPeriodUsage" ||
		got.Header.Get("Authorization") != "Bearer tok" || got.Header.Get("Connect-Protocol-Version") != "1" {
		t.Errorf("request %s %s %v", got.Method, got.URL.Path, got.Header)
	}
	status = http.StatusUnauthorized
	if _, err := FetchCursorUsage(context.Background(), srv.Client(), srv.URL, "tok"); err == nil {
		t.Error("401 fetched, want an error")
	}
}

// TestCursorUsageAsksAtMostEveryInterval: the sampler asks every 2s; the API
// is called once an interval, out of the sample, and a failure keeps the last
// reading.
func TestCursorUsageAsksAtMostEveryInterval(t *testing.T) {
	calls := make(chan struct{}, 10)
	fail := false
	u := &CursorUsage{
		Every: time.Minute,
		Token: func(context.Context) (string, error) { return "tok", nil },
		Fetch: func(_ context.Context, token string) ([]byte, error) {
			calls <- struct{}{}
			if fail {
				return nil, context.DeadlineExceeded
			}
			return []byte(cursorUsage), nil
		},
	}
	now := time.UnixMilli(1791000000000)
	if r := u.Reading(now); r != nil {
		t.Fatalf("the first sample waits for no call: %+v", r)
	}
	<-calls
	u.Wait()
	r := u.Reading(now.Add(time.Second))
	if r == nil || r.Windows[0].UsedPct != 25 {
		t.Fatalf("reading %+v, want the month's 25%%", r)
	}
	if len(calls) != 0 {
		t.Errorf("%d calls within the interval, want none", len(calls))
	}
	fail = true
	u.Reading(now.Add(2 * time.Minute))
	<-calls
	u.Wait()
	if r := u.Reading(now.Add(2*time.Minute + time.Second)); r == nil || r.Windows[0].UsedPct != 25 {
		t.Errorf("after a failed call: %+v, want the last reading", r)
	}
}

func TestCursorUsageWithoutATokenAsksNothing(t *testing.T) {
	u := &CursorUsage{
		Every: time.Minute,
		Token: func(context.Context) (string, error) { return "", context.Canceled },
		Fetch: func(context.Context, string) ([]byte, error) {
			t.Error("called the API with no token")
			return nil, nil
		},
	}
	u.Reading(time.Now())
	u.Wait()
	if r := u.Reading(time.Now()); r != nil {
		t.Errorf("reading %+v, want none", r)
	}
}

// TestEachCursorAccountHasItsOwnReading: each account folder is read with
// its own token, and the default with the keychain's.
func TestEachCursorAccountHasItsOwnReading(t *testing.T) {
	plan := func(pct string) []byte {
		return []byte(`{"billingCycleStart":"1790000000000","billingCycleEnd":"1792678400000","planUsage":{"totalPercentUsed":` + pct + `}}`)
	}
	answers := map[string][]byte{"tok-a": plan("10"), "tok-b": plan("20"), "tok-keychain": plan("30")}
	u := &CursorUsages{
		Every:    time.Hour,
		Keychain: func(context.Context) (string, error) { return "tok-keychain", nil },
		File: func(path string) (string, error) {
			return map[string]string{
				"/work/a/home/.cursor/auth.json": "tok-a",
				"/work/b/home/.cursor/auth.json": "tok-b",
			}[path], nil
		},
		Fetch: func(_ context.Context, token string) ([]byte, error) { return answers[token], nil },
	}
	now := time.UnixMilli(1791000000000)
	for _, dir := range []string{"/work/a", "/work/b", ""} {
		u.Reading(dir, now)
	}
	u.Wait()
	for dir, want := range map[string]float64{"/work/a": 10, "/work/b": 20, "": 30} {
		if r := u.Reading(dir, now.Add(time.Second)); r == nil || r.Windows[0].UsedPct != want {
			t.Errorf("%q: reading %+v, want %v%%", dir, r, want)
		}
	}
}

// TestFileTokenReadsOnlyTheToken: of Cursor's auth.json the access token is
// read, and a missing file is an error.
func TestFileTokenReadsOnlyTheToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"accessToken":"acc","refreshToken":"ref"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := FileToken(path); err != nil || got != "acc" {
		t.Errorf("token %q, %v, want acc", got, err)
	}
	if _, err := FileToken(path + ".missing"); err == nil {
		t.Error("a missing file read, want an error")
	}
}

// TestCursorAccountNotSignedInAsksNothing: an account folder without an
// auth.json has no reading and no call.
func TestCursorAccountNotSignedInAsksNothing(t *testing.T) {
	u := &CursorUsages{
		Every: time.Hour,
		File:  FileToken,
		Fetch: func(context.Context, string) ([]byte, error) {
			t.Error("called the API with no login")
			return nil, nil
		},
	}
	dir := t.TempDir()
	u.Reading(dir, time.Now())
	u.Wait()
	if r := u.Reading(dir, time.Now()); r != nil {
		t.Errorf("reading %+v, want none", r)
	}
}

// TestCursorSignedOutIsAskedAgainEvery30Seconds: a login with no token is
// read again after 30 s, not at each sample and not after the interval: a
// sign-in at t0+1s is fetched at t0+30s, and none before.
func TestCursorSignedOutIsAskedAgainEvery30Seconds(t *testing.T) {
	signedIn := false
	reads, calls := 0, 0
	u := &CursorUsages{
		Every: 5 * time.Minute,
		File: func(string) (string, error) {
			reads++
			if !signedIn {
				return "", os.ErrNotExist
			}
			return "tok", nil
		},
		Fetch: func(context.Context, string) ([]byte, error) {
			calls++
			return []byte(cursorUsage), nil
		},
	}
	dir := "/home/u/.matchblox/accounts/cursor-agent/work"
	t0 := time.UnixMilli(1791000000000)
	sample := func(d time.Duration) *Reading {
		r := u.Reading(dir, t0.Add(d))
		u.Wait()
		return r
	}
	sample(0)
	if reads != 1 || u.Asked(dir) {
		t.Fatalf("%d reads (asked %v) at t0, want 1 and no call", reads, u.Asked(dir))
	}
	for d := time.Second; d < 30*time.Second; d += time.Second {
		if d == time.Second {
			signedIn = true
		}
		sample(d)
	}
	if reads != 1 || calls != 0 {
		t.Fatalf("before t0+30s: %d reads, %d calls, want 1 and 0", reads, calls)
	}
	sample(30 * time.Second)
	if reads != 2 || calls != 1 || !u.Asked(dir) {
		t.Fatalf("at t0+30s: %d reads, %d calls, want 2 and 1", reads, calls)
	}
	if r := sample(31 * time.Second); r == nil || r.Windows[0].UsedPct != 25 {
		t.Errorf("reading %+v, want the month's 25%%", r)
	}
	// Signed in: the interval applies again.
	sample(4 * time.Minute)
	if reads != 2 || calls != 1 {
		t.Errorf("within the interval: %d reads, %d calls, want 2 and 1", reads, calls)
	}
}

// TestCursorSignedOutIsReadOncePer30Seconds: while signed out, one token
// read per 30 s, however often it samples.
func TestCursorSignedOutIsReadOncePer30Seconds(t *testing.T) {
	reads := 0
	u := &CursorUsages{
		Every:    5 * time.Minute,
		Keychain: func(context.Context) (string, error) { reads++; return "", os.ErrNotExist },
		Fetch: func(context.Context, string) ([]byte, error) {
			t.Error("called the API with no token")
			return nil, nil
		},
	}
	t0 := time.UnixMilli(1791000000000)
	for d := time.Duration(0); d < 2*time.Minute; d += time.Second {
		u.Reading("", t0.Add(d))
		u.Wait()
	}
	if reads != 4 {
		t.Errorf("%d reads in 2 minutes, want 4", reads)
	}
}

// TestCursorTokenReadFailingIsNotSignedOut: a token read that fails for any
// reason but a missing login (a denied Keychain) is not signed out and is
// read again after the interval, not every 30 s.
func TestCursorTokenReadFailingIsNotSignedOut(t *testing.T) {
	reads := 0
	u := &CursorUsages{
		Every:    5 * time.Minute,
		Keychain: func(context.Context) (string, error) { reads++; return "", errors.New("security: exit status 51") },
		Fetch: func(context.Context, string) ([]byte, error) {
			t.Error("called the API with no token")
			return nil, nil
		},
	}
	t0 := time.UnixMilli(1791000000000)
	for d := time.Duration(0); d < 2*time.Minute; d += time.Second {
		u.Reading("", t0.Add(d))
		u.Wait()
	}
	if reads != 1 {
		t.Errorf("%d reads in 2 minutes, want 1", reads)
	}
	if u.SignedOut("") {
		t.Error("a failed read is signed out, want not")
	}
	if !u.ReadFailed("") {
		t.Error("a failed read did not fail, want failed")
	}
}

// TestCursorTokenReadThatWorksIsNotFailed: a missing login and a token read
// are not a failed read, and a read that works again clears one.
func TestCursorTokenReadThatWorksIsNotFailed(t *testing.T) {
	var err error
	token := ""
	u := &CursorUsages{
		Every:    time.Minute,
		Keychain: func(context.Context) (string, error) { return token, err },
		Fetch:    func(context.Context, string) ([]byte, error) { return nil, context.DeadlineExceeded },
	}
	t0 := time.UnixMilli(1791000000000)
	steps := []struct {
		token string
		err   error
		want  bool
	}{
		{"", os.ErrNotExist, false},
		{"", errors.New("security: exit status 51"), true},
		{"tok", nil, false},
	}
	for i, st := range steps {
		token, err = st.token, st.err
		u.Reading("", t0.Add(time.Duration(i)*time.Hour))
		u.Wait()
		if got := u.ReadFailed(""); got != st.want {
			t.Errorf("step %d (%q, %v): failed %v, want %v", i, st.token, st.err, got, st.want)
		}
	}
}

// TestCursorAuthJSONHalfWrittenIsRetriedIn30s: an auth.json that is half
// written is a login not there yet, not a failed read: it is read again after
// 30 s, and the full file written meanwhile is used.
func TestCursorAuthJSONHalfWrittenIsRetriedIn30s(t *testing.T) {
	u := &CursorUsages{
		Every: 5 * time.Minute,
		File:  FileToken,
		Fetch: func(context.Context, string) ([]byte, error) { return []byte(cursorUsage), nil },
	}
	dir := t.TempDir()
	path := filepath.Join(dir, cursorAuthFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"accessTok`), 0o600); err != nil {
		t.Fatal(err)
	}
	t0 := time.UnixMilli(1791000000000)
	u.Reading(dir, t0)
	u.Wait()
	if u.ReadFailed(dir) {
		t.Error("a half-written auth.json is a failed read, want a login not there yet")
	}
	if !u.SignedOut(dir) {
		t.Error("a half-written auth.json is not signed out, want signed out")
	}
	if err := os.WriteFile(path, []byte(`{"accessToken":"acc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	u.Reading(dir, t0.Add(30*time.Second))
	u.Wait()
	if !u.Asked(dir) {
		t.Error("the full file written 30 s later was not read, want a call")
	}
}
