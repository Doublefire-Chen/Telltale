package dashboard_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type managedHost struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Hidden bool   `json:"hidden"`
	Report *struct {
		CPUPercent *float64 `json:"cpu_percent"`
	} `json:"report"`
}

func (h *harness) managed(session *http.Cookie) []managedHost {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var body struct {
		Hosts []managedHost `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		h.t.Fatal(err)
	}
	return body.Hosts
}

func (h *harness) patchHost(session *http.Cookie, id string, fields map[string]any) *http.Response {
	h.t.Helper()
	body, _ := json.Marshal(fields)
	return h.do(http.MethodPatch, "/api/hosts/"+id, string(body), session)
}

func names(hosts []managedHost) string {
	var out []string
	for _, h := range hosts {
		out = append(out, h.Name)
	}
	return strings.Join(out, ",")
}

func TestRenameGroupAndHide(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "old-name")

	expectStatus(t, h.patchHost(session, host.ID, map[string]any{"name": "  Tokyo  ", "group": " Asia ", "hidden": true}), http.StatusNoContent)
	got := h.managed(session)[0]
	if got.Name != "Tokyo" || got.Group != "Asia" || !got.Hidden {
		t.Fatalf("after update: %+v", got)
	}

	// Fields left out stay as they are.
	expectStatus(t, h.patchHost(session, host.ID, map[string]any{"hidden": false}), http.StatusNoContent)
	if got := h.managed(session)[0]; got.Name != "Tokyo" || got.Group != "Asia" || got.Hidden {
		t.Fatalf("partial update changed other fields: %+v", got)
	}
	// A blank name falls back to the hostname; a blank group ungroups.
	expectStatus(t, h.patchHost(session, host.ID, map[string]any{"name": "", "group": ""}), http.StatusNoContent)
	if got := h.managed(session)[0]; got.Name != "" || got.Group != "" {
		t.Fatalf("clearing: %+v", got)
	}

	expectError(t, h.patchHost(session, host.ID, map[string]any{"name": strings.Repeat("x", 65)}), http.StatusBadRequest, "invalid_name")
	expectError(t, h.patchHost(session, host.ID, map[string]any{"group": strings.Repeat("x", 33)}), http.StatusBadRequest, "invalid_group")
	expectError(t, h.patchHost(session, "missing", map[string]any{"name": "x"}), http.StatusNotFound, "not_found")
	expectError(t, h.patchHost(nil, host.ID, map[string]any{"name": "x"}), http.StatusUnauthorized, "unauthenticated")
}

func TestReorder(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	a, b, c := h.addHost(session, "a"), h.addHost(session, "b"), h.addHost(session, "c")
	if got := names(h.managed(session)); got != "a,b,c" {
		t.Fatalf("initial order %s", got)
	}

	order := func(ids ...string) *http.Response {
		body, _ := json.Marshal(map[string]any{"ids": ids})
		return h.do(http.MethodPut, "/api/hosts/order", string(body), session)
	}
	expectStatus(t, order(c.ID, a.ID, b.ID), http.StatusNoContent)
	if got := names(h.managed(session)); got != "c,a,b" {
		t.Fatalf("after reorder: %s", got)
	}
	// New Hosts go to the end.
	h.addHost(session, "d")
	if got := names(h.managed(session)); got != "c,a,b,d" {
		t.Fatalf("after adding: %s", got)
	}

	for name, ids := range map[string][]string{
		"missing one": {c.ID, a.ID, b.ID},
		"duplicate":   {c.ID, a.ID, a.ID, b.ID},
		"unknown":     {c.ID, a.ID, b.ID, "nope"},
	} {
		if resp := order(ids...); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
	if got := names(h.managed(session)); got != "c,a,b,d" {
		t.Fatalf("a rejected order changed something: %s", got)
	}
}

func TestRegenerateToken(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	const report = `{"time":1,"interval_ms":2000}`
	expectStatus(t, h.postReport("Bearer "+host.Token, report), http.StatusNoContent)

	resp := h.do(http.MethodPost, "/api/hosts/"+host.ID+"/token", "", session)
	expectStatus(t, resp, http.StatusOK)
	var body struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if !strings.HasPrefix(body.Token, "tt_") || body.Token == host.Token {
		t.Fatalf("new token %q", body.Token)
	}

	expectStatus(t, h.postReport("Bearer "+host.Token, report), http.StatusUnauthorized)
	expectStatus(t, h.postReport("Bearer "+body.Token, report), http.StatusNoContent)
	// The new token is not served anywhere else.
	raw, _ := json.Marshal(h.managed(session))
	if strings.Contains(string(raw), body.Token) {
		t.Fatal("status exposes the regenerated token")
	}
	expectError(t, h.do(http.MethodPost, "/api/hosts/missing/token", "", session), http.StatusNotFound, "not_found")
	expectError(t, h.do(http.MethodPost, "/api/hosts/"+host.ID+"/token", "", nil), http.StatusUnauthorized, "unauthenticated")
}

func TestDeleteHost(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	gone := h.addHost(session, "gone")
	kept := h.addHost(session, "kept")
	expectStatus(t, h.postReport("Bearer "+gone.Token, `{"time":1,"interval_ms":2000,"cpu_percent":9}`), http.StatusNoContent)

	expectStatus(t, h.do(http.MethodDelete, "/api/hosts/"+gone.ID, "", session), http.StatusNoContent)
	if got := names(h.managed(session)); got != "kept" {
		t.Fatalf("after delete: %s", got)
	}
	// Its token no longer works, and re-adding starts from nothing.
	expectStatus(t, h.postReport("Bearer "+gone.Token, `{"time":2,"interval_ms":2000}`), http.StatusUnauthorized)
	if hosts := h.managed(session); hosts[0].ID != kept.ID || hosts[0].Report != nil {
		t.Fatalf("remaining Host: %+v", hosts[0])
	}
	expectError(t, h.do(http.MethodDelete, "/api/hosts/"+gone.ID, "", session), http.StatusNotFound, "not_found")
	expectError(t, h.do(http.MethodDelete, "/api/hosts/"+kept.ID, "", nil), http.StatusUnauthorized, "unauthenticated")
}
