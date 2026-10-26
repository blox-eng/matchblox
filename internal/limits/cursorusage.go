package limits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// CursorAPI is where Cursor's CLI asks for its plan usage.
const CursorAPI = "https://api2.cursor.sh"

// cursorUsagePath is GetCurrentPeriodUsage of Cursor's DashboardService, in
// the Connect protocol: a POST of JSON.
const cursorUsagePath = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"

// FetchCursorUsage asks Cursor's API for the plan usage of the login token
// is of, as Cursor's CLI does for /usage. The token goes in the
// Authorization header and nowhere else.
func FetchCursorUsage(ctx context.Context, client *http.Client, api, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api+cursorUsagePath, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() // a body read to its end loses nothing on close
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursor usage: %s", resp.Status)
	}
	return b, nil
}

// ParseCursorUsage reads GetCurrentPeriodUsage: the month of the plan is one
// window, the share of the included usage used until the billing cycle
// ends, as /usage shows it under Included.
func ParseCursorUsage(b []byte, now time.Time) (*Reading, error) {
	var f struct {
		Start string `json:"billingCycleStart"` // unix milliseconds, as a string
		End   string `json:"billingCycleEnd"`
		Plan  *struct {
			Used float64 `json:"totalPercentUsed"`
		} `json:"planUsage"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Plan == nil {
		return nil, errors.New("cursor usage: no plan usage")
	}
	start, _ := strconv.ParseInt(f.Start, 10, 64)
	end, _ := strconv.ParseInt(f.End, 10, 64)
	w := Window{UsedPct: f.Plan.Used}
	if end > 0 {
		w.ResetsAt = time.UnixMilli(end)
		if start > 0 && end > start {
			w.Minutes = int(time.Duration(end-start) * time.Millisecond / time.Minute)
		}
	}
	return &Reading{At: now, Windows: []Window{w}}, nil
}

// CursorUsage keeps Cursor's plan usage, asked of its API at most once an
// interval and never in the caller's time: Reading returns the last one and
// asks again in the background when it is due. A failed call keeps the last
// reading. A read with no token is no call: a missing login (an error that
// is os.ErrNotExist, or no token) is read again after cursorRetry; a read
// that failed otherwise, and a token read, after Every.
type CursorUsage struct {
	Every time.Duration
	// Token is Cursor's login token, read when a call is due and dropped
	// after it.
	Token func(context.Context) (string, error)
	Fetch func(ctx context.Context, token string) ([]byte, error)

	mu     sync.Mutex
	last   *Reading
	asked  time.Time
	called bool // a token was read and the API asked
	// signedOut: the last read found no token; the next is due after
	// cursorRetry.
	signedOut bool
	// failed: the last read ended in an error other than a missing login (a
	// denied Keychain); no call was made.
	failed bool
	wg     sync.WaitGroup
}

// cursorCallTimeout bounds one call: the token read and the API's answer.
const cursorCallTimeout = 15 * time.Second

// cursorRetry is how soon a login with no token is read again: a sign-in
// shows within it, and the Keychain is not read at each sample.
const cursorRetry = 30 * time.Second

func (u *CursorUsage) Reading(now time.Time) *Reading {
	u.mu.Lock()
	defer u.mu.Unlock()
	every := u.Every
	if u.signedOut {
		every = min(every, cursorRetry)
	}
	if u.asked.IsZero() || now.Sub(u.asked) >= every {
		u.asked = now
		u.wg.Add(1)
		go u.ask(now)
	}
	return u.last
}

func (u *CursorUsage) ask(now time.Time) {
	defer u.wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), cursorCallTimeout)
	defer cancel()
	token, err := u.Token(ctx)
	u.mu.Lock()
	// Only a missing login is signed out; a read that failed otherwise (a
	// denied Keychain) waits the interval.
	u.signedOut = token == "" && (err == nil || errors.Is(err, os.ErrNotExist))
	u.failed = err != nil && !errors.Is(err, os.ErrNotExist)
	if err == nil && token != "" {
		u.called = true
	}
	u.mu.Unlock()
	if err != nil || token == "" {
		return
	}
	b, err := u.Fetch(ctx, token)
	if err != nil {
		return
	}
	r, err := ParseCursorUsage(b, now)
	if err != nil {
		return
	}
	u.mu.Lock()
	u.last = r
	u.mu.Unlock()
}

// Wait waits for a call in the background to end.
func (u *CursorUsage) Wait() { u.wg.Wait() }

// Asked says whether a call was made: a token was read and the API asked.
func (u *CursorUsage) Asked() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.called
}

// SignedOut says whether the last token read finished with no token. A
// read still running is not signed out.
func (u *CursorUsage) SignedOut() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.signedOut
}

// ReadFailed says whether the last token read ended in an error other than a
// missing login (a denied Keychain). Such a read made no call.
func (u *CursorUsage) ReadFailed() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.failed
}

// CursorUsages keeps one CursorUsage for each Cursor login: the default's,
// whose token is in the keychain, and each account folder's, whose token is
// in the auth.json of its file store. A reading is never handed to another
// login.
type CursorUsages struct {
	Every time.Duration
	// Keychain reads the default login's token; nil: the default is not read.
	// A missing login is an error that is os.ErrNotExist.
	Keychain func(context.Context) (string, error)
	// File reads the token of the auth.json at path; nil: accounts are not
	// read.
	File  func(path string) (string, error)
	Fetch func(ctx context.Context, token string) ([]byte, error)

	mu   sync.Mutex
	byID map[string]*CursorUsage // by account folder; "" is the default
}

// cursorAuthFile is where Cursor's file store keeps the login of an account
// folder.
const cursorAuthFile = "home/.cursor/auth.json"

// Reads says whether the login of dir ("" is the default) is read at all.
func (u *CursorUsages) Reads(dir string) bool {
	if dir == "" {
		return u.Keychain != nil
	}
	return u.File != nil
}

// Reading is the last plan usage of the login of dir, the account folder, or
// of the default for "". A login that is not read, or has no token (an
// account not signed in yet), has no reading and no call.
func (u *CursorUsages) Reading(dir string, now time.Time) *Reading {
	if !u.Reads(dir) {
		return nil
	}
	u.mu.Lock()
	c, ok := u.byID[dir]
	if !ok {
		c = &CursorUsage{Every: u.Every, Fetch: u.Fetch}
		if dir == "" {
			c.Token = u.Keychain
		} else {
			path := filepath.Join(dir, cursorAuthFile)
			c.Token = func(context.Context) (string, error) { return u.File(path) }
		}
		if u.byID == nil {
			u.byID = map[string]*CursorUsage{}
		}
		u.byID[dir] = c
	}
	u.mu.Unlock()
	return c.Reading(now)
}

// Asked says whether a call was made for the login of dir ("" is the
// default): a token was read and the API asked.
func (u *CursorUsages) Asked(dir string) bool {
	u.mu.Lock()
	c, ok := u.byID[dir]
	u.mu.Unlock()
	return ok && c.Asked()
}

// SignedOut says whether the last token read of the login of dir ("" is the
// default) finished with no token. A read still running is not signed out.
func (u *CursorUsages) SignedOut(dir string) bool {
	u.mu.Lock()
	c, ok := u.byID[dir]
	u.mu.Unlock()
	return ok && c.SignedOut()
}

// ReadFailed says whether the last token read of the login of dir ("" is
// the default) ended in an error other than a missing login. Such a read made
// no call.
func (u *CursorUsages) ReadFailed(dir string) bool {
	u.mu.Lock()
	c, ok := u.byID[dir]
	u.mu.Unlock()
	return ok && c.ReadFailed()
}

// Wait waits for the calls in the background to end.
func (u *CursorUsages) Wait() {
	u.mu.Lock()
	all := make([]*CursorUsage, 0, len(u.byID))
	for _, c := range u.byID {
		all = append(all, c)
	}
	u.mu.Unlock()
	for _, c := range all {
		c.Wait()
	}
}

// FileToken reads the access token of Cursor's auth.json, and nothing else
// of it: the refresh token is never held. A file that is not JSON (half
// written) is a login not there yet: the error is os.ErrNotExist.
func FileToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var f struct {
		Access string `json:"accessToken"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return "", fmt.Errorf("cursor auth: not JSON: %w", os.ErrNotExist)
	}
	return f.Access, nil
}
