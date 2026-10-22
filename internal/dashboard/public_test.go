package dashboard_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func (h *harness) setPublic(session *http.Cookie, public bool) {
	h.t.Helper()
	body, _ := json.Marshal(map[string]bool{"status_public": public})
	expectStatus(h.t, h.do(http.MethodPut, "/api/settings", string(body), session), http.StatusNoContent)
}

func (h *harness) rawStatus(session *http.Cookie) string {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// publicFixture has a visible Host and a Hidden one, both with addresses and Reports.
func publicFixture(t *testing.T) (h *harness, session *http.Cookie, visible, hidden addedHost) {
	t.Helper()
	h, handler := addressHarness(t, nil)
	h.setPassword(testPassword)
	session = h.loginOK()
	visible = h.addHost(session, "visible-box")
	hidden = h.addHost(session, "secret-box")
	expectStatus(t, h.patchHost(session, hidden.ID, map[string]any{"hidden": true}), http.StatusNoContent)
	expectStatus(t, h.patchHost(session, visible.ID, map[string]any{"country_override": "SE"}), http.StatusNoContent)
	hostInfoFrom(t, handler, visible.Token, "203.0.113.7:4000", nil)
	hostInfoFrom(t, handler, visible.Token, "[2001:db8:1::7]:4000", nil)
	hostInfoFrom(t, handler, hidden.Token, "198.51.100.9:4000", nil)
	for _, tok := range []string{visible.Token, hidden.Token} {
		expectStatus(t, h.postReport("Bearer "+tok, `{"time":1,"interval_ms":2000,"cpu_percent":1}`), http.StatusNoContent)
	}
	return h, session, visible, hidden
}

func TestPrivateByDefaultRefusesAnonymous(t *testing.T) {
	h, session, _, _ := publicFixture(t)

	resp := h.do(http.MethodGet, "/api/settings", "", session)
	var settings struct {
		StatusPublic bool `json:"status_public"`
	}
	json.NewDecoder(resp.Body).Decode(&settings)
	if settings.StatusPublic {
		t.Fatal("Status page is public by default")
	}
	expectError(t, h.do(http.MethodGet, "/api/status", "", nil), http.StatusUnauthorized, "unauthenticated")
	if _, pub := sessionPublic(t, h); pub {
		t.Fatal("session reports a public Status page")
	}
	// Only the admin can change it.
	expectError(t, h.do(http.MethodPut, "/api/settings", `{"status_public":true}`, nil), http.StatusUnauthorized, "unauthenticated")
}

func sessionPublic(t *testing.T, h *harness) (authenticated, public bool) {
	t.Helper()
	resp := h.do(http.MethodGet, "/api/session", "", nil)
	var body struct {
		Authenticated bool `json:"authenticated"`
		StatusPublic  bool `json:"status_public"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	return body.Authenticated, body.StatusPublic
}

func TestPublicStatusHidesIPsAndHiddenHosts(t *testing.T) {
	h, session, visible, _ := publicFixture(t)
	h.setPublic(session, true)
	if auth, pub := sessionPublic(t, h); auth || !pub {
		t.Fatalf("anonymous session: authenticated=%v public=%v", auth, pub)
	}

	anon := h.rawStatus(nil)
	for _, leak := range []string{"secret-box", "203.0.113.7", "2001:db8:1::7", "198.51.100.9", `"country_override":"SE"`} {
		if strings.Contains(anon, leak) {
			t.Errorf("anonymous status contains %q:\n%s", leak, anon)
		}
	}
	var body struct {
		Hosts []struct {
			ID        string   `json:"id"`
			Countries []string `json:"countries"`
			Report    *struct {
				CPUPercent float64 `json:"cpu_percent"`
			} `json:"report"`
		} `json:"hosts"`
	}
	json.Unmarshal([]byte(anon), &body)
	if len(body.Hosts) != 1 || body.Hosts[0].ID != visible.ID || body.Hosts[0].Report == nil {
		t.Fatalf("anonymous viewers should see the visible Host with its metrics: %s", anon)
	}
	if strings.Join(body.Hosts[0].Countries, ",") != "SE" {
		t.Fatalf("flags should stay public: %v", body.Hosts[0].Countries)
	}

	// The admin still sees everything, in either mode.
	for _, public := range []bool{true, false} {
		h.setPublic(session, public)
		admin := h.rawStatus(session)
		for _, want := range []string{"secret-box", "203.0.113.7", "198.51.100.9", `"country_override":"SE"`} {
			if !strings.Contains(admin, want) {
				t.Errorf("public=%v: admin status lacks %q", public, want)
			}
		}
	}
	expectError(t, h.do(http.MethodGet, "/api/status", "", nil), http.StatusUnauthorized, "unauthenticated")
}

func TestPublicLiveFeedWithholdsHiddenHosts(t *testing.T) {
	h, session, visible, hidden := publicFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/api/live"

	// Private: no anonymous live feed.
	if _, resp, err := websocket.Dial(ctx, wsURL, nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous dial while private: %v", err)
	}

	h.setPublic(session, true)
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	// A Hidden Host's Report is withheld; the visible Host's arrives.
	expectStatus(t, h.postReport("Bearer "+hidden.Token, `{"time":2,"interval_ms":2000,"cpu_percent":66}`), http.StatusNoContent)
	expectStatus(t, h.postReport("Bearer "+visible.Token, `{"time":2,"interval_ms":2000,"cpu_percent":33}`), http.StatusNoContent)
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), hidden.ID) || !strings.Contains(string(data), visible.ID) {
		t.Fatalf("first anonymous live message: %s", data)
	}

	// Going private disconnects anonymous viewers at once.
	h.setPublic(session, false)
	readCtx, cancelRead := context.WithTimeout(ctx, time.Second)
	defer cancelRead()
	for {
		if _, _, err := c.Read(readCtx); err != nil {
			if readCtx.Err() != nil {
				t.Fatal("anonymous viewer still connected after going private")
			}
			break
		}
	}
}
