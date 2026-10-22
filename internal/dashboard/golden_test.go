package dashboard_test

import (
	"net/http"
	"testing"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
	"github.com/Doublefire-Chen/Telltale/internal/protocol/v1/golden"
)

// TestAcceptsEveryReleasedAgent posts what each released Agent sends to the
// Dashboard's ingest API: a Dashboard upgrade must never shut out an old
// Agent (ADR 0002).
func TestAcceptsEveryReleasedAgent(t *testing.T) {
	payloads, err := golden.All()
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	tokens := map[string]string{}
	for _, p := range payloads {
		token, ok := tokens[p.Version]
		if !ok {
			token = "Bearer " + h.addHost(session, p.Version).Token
			tokens[p.Version] = token
		}
		var resp *http.Response
		if p.Path == v1.ReportPath {
			resp = h.postReport(token, string(p.Body))
		} else {
			resp = h.postHostInfo(token, string(p.Body))
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Agent %s: %s answered %d, want 204", p.Version, p.Path, resp.StatusCode)
		}
	}

	_, hosts := h.hostsWithInfo(session)
	for _, host := range hosts {
		if host.HostInfo == nil || host.HostInfo["hostname"] == nil {
			t.Errorf("Agent %s: Host Info not shown", host.Name)
		}
	}
	for name, online := range h.onlineStates(session) {
		if !online {
			t.Errorf("Agent %s: Report not shown", name)
		}
	}
}
