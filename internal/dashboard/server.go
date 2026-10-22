// Package dashboard implements the Dashboard's HTTP server: the web app, the
// session and Admin area API, the Status page API with its live feed, and the
// protocol v1 endpoints Agents report to.
package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
	"github.com/Doublefire-Chen/Telltale/internal/version"
)

// AdminUsername is the only account's fixed username.
const AdminUsername = "admin"

const (
	sessionCookie   = "telltale_session"
	sessionLifetime = 30 * 24 * time.Hour

	loginMaxFailures = 5
	loginWindow      = 15 * time.Minute

	// Agents retry every interval, so a few rejections are normal; a burst
	// from one address is someone guessing tokens.
	agentMaxFailures   = 20
	agentFailureWindow = time.Minute

	maxJSONBody = 4 << 10
)

// Options configures a Server.
type Options struct {
	Store *store.Store
	// App is the built web app; nil serves a placeholder page.
	App fs.FS
	// Now returns the current time; defaults to time.Now. Tests inject a fake clock.
	Now    func() time.Time
	Logger *slog.Logger
	// Version is this Dashboard's release version; defaults to version.Version.
	Version string
	// TrustedProxies are the reverse proxies whose forwarded-for headers are
	// believed; nil means DefaultTrustedProxies.
	TrustedProxies []netip.Prefix
	// GeoIP resolves countries; nil leaves them unknown.
	GeoIP *geoip.DB
	// SweepInterval is how often Run looks for Hosts that went Offline;
	// defaults to one second.
	SweepInterval time.Duration
}

// Server is the Dashboard's HTTP handler.
type Server struct {
	store          *store.Store
	app            fs.FS
	version        string
	trustedProxies []netip.Prefix
	geoip          *geoip.DB
	now            func() time.Time
	logger         *slog.Logger
	logins         *failureLimiter
	// agentFailures rate-limits rejected Agent requests per client address.
	agentFailures *failureLimiter
	latest        latestReports
	hub           hub
	mux           *http.ServeMux

	sweepEvery time.Duration
	// offline holds the Hosts last announced as Offline; guarded by offlineMu.
	offlineMu sync.Mutex
	offline   map[string]bool
}

// New builds a Server from opts.
func New(opts Options) *Server {
	s := &Server{
		store:         opts.Store,
		app:           opts.App,
		now:           opts.Now,
		logger:        opts.Logger,
		logins:        newFailureLimiter(loginMaxFailures, loginWindow),
		agentFailures: newFailureLimiter(agentMaxFailures, agentFailureWindow),
		mux:           http.NewServeMux(),
		sweepEvery:    opts.SweepInterval,
		offline:       make(map[string]bool),
	}
	if s.sweepEvery <= 0 {
		s.sweepEvery = time.Second
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	s.version = opts.Version
	if s.version == "" {
		s.version = version.Version
	}
	s.trustedProxies = opts.TrustedProxies
	if s.trustedProxies == nil {
		s.trustedProxies = DefaultTrustedProxies
	}
	s.geoip = opts.GeoIP

	s.mux.HandleFunc("GET /api/session", s.handleGetSession)
	s.mux.Handle("POST /api/session", s.requireSetup(http.HandlerFunc(s.handleLogin)))
	s.mux.Handle("DELETE /api/session", s.requireSetup(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("GET /api/status", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleStatus))))
	s.mux.Handle("GET /api/live", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleLive))))
	s.mux.Handle("POST /api/hosts", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleAddHost))))
	s.mux.Handle("PATCH /api/hosts/{id}", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleUpdateHost))))
	s.mux.Handle("DELETE /api/hosts/{id}", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleDeleteHost))))
	s.mux.Handle("POST /api/hosts/{id}/token", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleRegenerateToken))))
	s.mux.Handle("PUT /api/hosts/order", s.requireSetup(s.requireAdmin(http.HandlerFunc(s.handleSetOrder))))
	s.mux.HandleFunc("POST "+v1.ReportPath, s.handleReport)
	s.mux.HandleFunc("POST "+v1.HostInfoPath, s.handleHostInfo)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found")
	})
	s.mux.HandleFunc("/", s.handleApp)
	return s
}

// Run does the Dashboard's background work until ctx ends: it announces
// Hosts that go Offline to the Status page.
func (s *Server) Run(ctx context.Context) {
	ticker := time.NewTicker(s.sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOffline()
		}
	}
}

// sweepOffline publishes an "offline" message for each Host that has missed
// three intervals since it was last announced online.
func (s *Server) sweepOffline() {
	now := s.now()
	var newlyOffline []string
	s.offlineMu.Lock()
	s.latest.each(func(id string, r reportView) {
		if !online(r, now) && !s.offline[id] {
			s.offline[id] = true
			newlyOffline = append(newlyOffline, id)
		}
	})
	s.offlineMu.Unlock()
	for _, id := range newlyOffline {
		s.hub.publish(liveMessage{Type: "offline", HostID: id})
	}
}

// markOnline records that a Host reported, so a later lapse is announced again.
func (s *Server) markOnline(hostID string) {
	s.offlineMu.Lock()
	delete(s.offline, hostID)
	s.offlineMu.Unlock()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'self'; img-src 'self' data:; style-src 'self'; connect-src 'self'; "+
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	s.mux.ServeHTTP(w, r)
}

// setupRequired reports whether no admin password has been set yet.
func (s *Server) setupRequired(ctx context.Context) (bool, error) {
	hash, err := s.store.AdminPasswordHash(ctx)
	return hash == "", err
}

// requireSetup refuses every request until an admin password exists.
func (s *Server) requireSetup(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		required, err := s.setupRequired(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if required {
			writeError(w, http.StatusServiceUnavailable, "setup_required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin refuses requests without a valid admin session.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.authenticated(r)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticated(r *http.Request) (bool, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false, nil
	}
	return s.store.SessionValid(r.Context(), hashToken(c.Value), s.now())
}

type sessionState struct {
	SetupRequired bool `json:"setup_required"`
	Authenticated bool `json:"authenticated"`
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	required, err := s.setupRequired(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	state := sessionState{SetupRequired: required}
	if !required {
		if state.Authenticated, err = s.authenticated(r); err != nil {
			s.internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, state)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	key := s.clientIP(r).String()
	now := s.now()
	if blocked, wait := s.logins.blocked(key, now); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second).Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}

	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	hash, err := s.store.AdminPasswordHash(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	match, err := VerifyPassword(req.Password, hash)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if req.Username != AdminUsername || !match {
		s.logins.fail(key, now)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	s.logins.reset(key)

	token, err := newToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	expires := now.Add(sessionLifetime)
	if err := s.store.CreateSession(r.Context(), hashToken(token), expires); err != nil {
		s.internalError(w, err)
		return
	}
	if err := s.store.DeleteExpiredSessions(r.Context(), now); err != nil {
		s.logger.Warn("delete expired sessions", "err", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.store.DeleteSession(r.Context(), hashToken(c.Value)); err != nil {
			s.internalError(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

const placeholderPage = `<!doctype html><meta charset="utf-8"><title>Telltale</title>
<p>The Telltale web app was not built into this binary. Run <code>npm run build</code> in <code>web/</code>, then rebuild the Dashboard.</p>`

// handleApp serves the single-page web app, falling back to index.html for client-side routes.
func (s *Server) handleApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.app == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, placeholderPage)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if info, err := fs.Stat(s.app, name); err != nil || info.IsDir() {
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.app, name)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal")
}

// decodeJSON reads a small JSON body into v, writing an error response and returning false on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "json_required")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "malformed")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
