package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

const testPassword = "correct horse battery"

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	t       *testing.T
	dataDir string
	store   *store.Store
	clock   *fakeClock
	srv     *httptest.Server
}

func newHarness(t *testing.T, app fstest.MapFS) *harness {
	t.Helper()
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	opts := dashboard.Options{Store: st, Now: clock.Now}
	if app != nil {
		opts.App = app
	}
	srv := httptest.NewServer(dashboard.New(opts))
	t.Cleanup(srv.Close)
	return &harness{t: t, dataDir: dataDir, store: st, clock: clock, srv: srv}
}

func (h *harness) setPassword(password string) {
	h.t.Helper()
	if err := dashboard.SetAdminPassword(context.Background(), h.store, password); err != nil {
		h.t.Fatal(err)
	}
}

// do sends a request, optionally with a JSON body and a session cookie.
func (h *harness) do(method, path, body string, session *http.Cookie) *http.Response {
	h.t.Helper()
	var r *http.Request
	var err error
	if body != "" {
		r, err = http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r, err = http.NewRequest(method, h.srv.URL+path, nil)
	}
	if err != nil {
		h.t.Fatal(err)
	}
	if session != nil {
		r.AddCookie(session)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (h *harness) login(username, password string) *http.Response {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	return h.do(http.MethodPost, "/api/session", string(body), nil)
}

// loginOK logs in as admin and returns the session cookie.
func (h *harness) loginOK() *http.Cookie {
	h.t.Helper()
	resp := h.login(dashboard.AdminUsername, testPassword)
	expectStatus(h.t, resp, http.StatusNoContent)
	for _, c := range resp.Cookies() {
		if c.Name == "telltale_session" {
			return c
		}
	}
	h.t.Fatal("login response set no session cookie")
	return nil
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want)
	}
}

func expectError(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	expectStatus(t, resp, status)
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error != code {
		t.Fatalf("error code %q, want %q", body.Error, code)
	}
}

func sessionState(t *testing.T, h *harness, session *http.Cookie) (setupRequired, authenticated bool) {
	t.Helper()
	resp := h.do(http.MethodGet, "/api/session", "", session)
	expectStatus(t, resp, http.StatusOK)
	var body struct {
		SetupRequired bool `json:"setup_required"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.SetupRequired, body.Authenticated
}

func TestSetupRequiredUntilPasswordSet(t *testing.T) {
	h := newHarness(t, nil)

	if setup, _ := sessionState(t, h, nil); !setup {
		t.Fatal("setup_required = false before any password was set")
	}
	expectError(t, h.login(dashboard.AdminUsername, testPassword), http.StatusServiceUnavailable, "setup_required")
	expectError(t, h.do(http.MethodGet, "/api/status", "", nil), http.StatusServiceUnavailable, "setup_required")

	h.setPassword(testPassword)
	if setup, auth := sessionState(t, h, nil); setup || auth {
		t.Fatalf("after set-password: setup_required=%v authenticated=%v, want false/false", setup, auth)
	}
}

func TestPrivateByDefault(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	expectError(t, h.do(http.MethodGet, "/api/status", "", nil), http.StatusUnauthorized, "unauthenticated")
	bogus := &http.Cookie{Name: "telltale_session", Value: "not-a-session"}
	expectError(t, h.do(http.MethodGet, "/api/status", "", bogus), http.StatusUnauthorized, "unauthenticated")
}

func TestLoginAndLogout(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	session := h.loginOK()
	if !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie flags: HttpOnly=%v Secure=%v SameSite=%v", session.HttpOnly, session.Secure, session.SameSite)
	}
	if _, auth := sessionState(t, h, session); !auth {
		t.Fatal("authenticated = false after login")
	}
	expectStatus(t, h.do(http.MethodGet, "/api/status", "", session), http.StatusOK)

	expectStatus(t, h.do(http.MethodDelete, "/api/session", "", session), http.StatusNoContent)
	// The session ends on the server, so replaying the old cookie fails.
	expectError(t, h.do(http.MethodGet, "/api/status", "", session), http.StatusUnauthorized, "unauthenticated")
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	expectError(t, h.login(dashboard.AdminUsername, "wrong password"), http.StatusUnauthorized, "invalid_credentials")
	expectError(t, h.login("root", testPassword), http.StatusUnauthorized, "invalid_credentials")
}

func TestLoginRequiresJSON(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	r, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/session", strings.NewReader("username=admin&password=x"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	expectError(t, resp, http.StatusUnsupportedMediaType, "json_required")
}

func TestLoginRateLimited(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	for range 5 {
		expectStatus(t, h.login(dashboard.AdminUsername, "wrong password"), http.StatusUnauthorized)
	}
	// Blocked even with the right password while the window lasts.
	resp := h.login(dashboard.AdminUsername, testPassword)
	expectError(t, resp, http.StatusTooManyRequests, "rate_limited")
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("rate-limited response has no Retry-After header")
	}

	h.clock.Advance(15*time.Minute + time.Second)
	h.loginOK()
}

func TestSuccessfulLoginResetsFailures(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)

	for range 4 {
		expectStatus(t, h.login(dashboard.AdminUsername, "wrong password"), http.StatusUnauthorized)
	}
	h.loginOK()
	for range 4 {
		expectStatus(t, h.login(dashboard.AdminUsername, "wrong password"), http.StatusUnauthorized)
	}
	h.loginOK()
}

func TestSessionExpires(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	h.clock.Advance(29 * 24 * time.Hour)
	expectStatus(t, h.do(http.MethodGet, "/api/status", "", session), http.StatusOK)
	h.clock.Advance(2 * 24 * time.Hour)
	expectError(t, h.do(http.MethodGet, "/api/status", "", session), http.StatusUnauthorized, "unauthenticated")
}

func TestPasswordResetEndsSessions(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	h.setPassword("another long password")
	expectError(t, h.do(http.MethodGet, "/api/status", "", session), http.StatusUnauthorized, "unauthenticated")
	expectStatus(t, h.login(dashboard.AdminUsername, testPassword), http.StatusUnauthorized)
	expectStatus(t, h.login(dashboard.AdminUsername, "another long password"), http.StatusNoContent)
}

func TestSetAdminPasswordValidatesLength(t *testing.T) {
	h := newHarness(t, nil)
	for _, pw := range []string{"short", strings.Repeat("x", dashboard.MaxPasswordLen+1)} {
		if err := dashboard.SetAdminPassword(context.Background(), h.store, pw); err == nil {
			t.Fatalf("SetAdminPassword accepted a %d-character password", len(pw))
		}
	}
	if setup, _ := sessionState(t, h, nil); !setup {
		t.Fatal("a rejected password was stored")
	}
}

func TestServesWebApp(t *testing.T) {
	app := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	h := newHarness(t, app)

	for _, path := range []string{"/", "/hosts/some-host"} {
		resp := h.do(http.MethodGet, path, "", nil)
		expectStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("GET %s: Content-Type %q, want text/html", path, ct)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Fatalf("GET %s: no Content-Security-Policy header", path)
		}
	}
	resp := h.do(http.MethodGet, "/assets/app-1.js", "", nil)
	expectStatus(t, resp, http.StatusOK)
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatal("hashed asset is not served as immutable")
	}
	expectStatus(t, h.do(http.MethodGet, "/assets/missing.js", "", nil), http.StatusNotFound)
	expectError(t, h.do(http.MethodGet, "/api/nope", "", nil), http.StatusNotFound, "not_found")
}

func TestServesPlaceholderWithoutWebApp(t *testing.T) {
	h := newHarness(t, nil)
	resp := h.do(http.MethodGet, "/", "", nil)
	expectStatus(t, resp, http.StatusOK)
}
