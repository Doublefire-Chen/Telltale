package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

func (h *harness) postHostInfo(authorization, body string) *http.Response {
	h.t.Helper()
	r, _ := http.NewRequest(http.MethodPost, h.srv.URL+v1.HostInfoPath, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

type hostWithInfo struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	HostInfo      map[string]any `json:"host_info"`
	AgentOutdated bool           `json:"agent_outdated"`
}

func (h *harness) hostsWithInfo(session *http.Cookie) (dashboardVersion string, hosts []hostWithInfo) {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var body struct {
		DashboardVersion string         `json:"dashboard_version"`
		Hosts            []hostWithInfo `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		h.t.Fatal(err)
	}
	return body.DashboardVersion, body.Hosts
}

const sampleHostInfo = `{"hostname":"vps-tokyo","os":"linux","platform":"ubuntu","platform_version":"24.04",` +
	`"kernel_version":"6.8.0-45-generic","arch":"x86_64","cpu_model":"AMD EPYC 7B13","cpu_cores":2,` +
	`"memory_total":2147483648,"swap_total":0,"disk_total":53687091200,"virtualization":"kvm",` +
	`"boot_time":1761000000,"agent_version":"v1.0.0","from_the_future":true}`

func TestHostInfoReachesStatusPage(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "")

	expectStatus(t, h.postHostInfo("Bearer "+host.Token, sampleHostInfo), http.StatusNoContent)
	_, hosts := h.hostsWithInfo(session)
	info := hosts[0].HostInfo
	if info == nil {
		t.Fatal("no Host Info on the Status page")
	}
	for field, want := range map[string]any{
		"hostname": "vps-tokyo", "platform": "ubuntu", "platform_version": "24.04", "arch": "x86_64",
		"cpu_cores": 2.0, "memory_total": 2147483648.0, "virtualization": "kvm", "boot_time": 1761000000.0,
	} {
		if info[field] != want {
			t.Errorf("%s = %v, want %v", field, info[field], want)
		}
	}
	if _, ok := info["from_the_future"]; ok {
		t.Error("unknown Host Info field was kept")
	}
	if hosts[0].Name != "" {
		t.Errorf("name = %q; a blank name stays blank so the hostname is shown", hosts[0].Name)
	}
}

func TestHostInfoSurvivesRestartAndUpdates(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "tokyo")
	expectStatus(t, h.postHostInfo("Bearer "+host.Token, sampleHostInfo), http.StatusNoContent)

	// A second Dashboard on the same database still knows the Host Info.
	other := httptest.NewServer(dashboard.New(dashboard.Options{Store: h.store, Now: h.clock.Now}))
	defer other.Close()
	h2 := &harness{t: t, store: h.store, clock: h.clock, srv: other}
	_, hosts := h2.hostsWithInfo(h2.loginOK())
	if hosts[0].HostInfo["hostname"] != "vps-tokyo" {
		t.Fatalf("Host Info lost across restart: %v", hosts[0].HostInfo)
	}

	updated := strings.Replace(sampleHostInfo, `"boot_time":1761000000`, `"boot_time":1761500000`, 1)
	expectStatus(t, h.postHostInfo("Bearer "+host.Token, updated), http.StatusNoContent)
	_, hosts = h.hostsWithInfo(session)
	if hosts[0].HostInfo["boot_time"] != 1761500000.0 {
		t.Fatalf("boot_time = %v after a reboot, want the new one", hosts[0].HostInfo["boot_time"])
	}
}

func TestHostInfoRejections(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "tokyo")

	expectStatus(t, h.postHostInfo("", sampleHostInfo), http.StatusUnauthorized)
	expectStatus(t, h.postHostInfo("Bearer tt_unknown", sampleHostInfo), http.StatusUnauthorized)
	expectStatus(t, h.postHostInfo("Bearer "+host.Token, `{"hostname":`), http.StatusBadRequest)
	if _, hosts := h.hostsWithInfo(session); hosts[0].HostInfo != nil {
		t.Fatal("rejected Host Info was stored")
	}
}

func TestAgentOutdatedBadge(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	clock := &fakeClock{now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	srv := httptest.NewServer(dashboard.New(dashboard.Options{Store: st, Now: clock.Now, Version: "v1.2.0"}))
	defer srv.Close()
	h := &harness{t: t, store: st, clock: clock, srv: srv}
	h.setPassword(testPassword)
	session := h.loginOK()

	for name, agentVersion := range map[string]string{"old": "v1.1.9", "same": "v1.2.0", "newer": "v1.3.0", "dev": "dev"} {
		host := h.addHost(session, name)
		info := strings.Replace(sampleHostInfo, `"agent_version":"v1.0.0"`, `"agent_version":"`+agentVersion+`"`, 1)
		expectStatus(t, h.postHostInfo("Bearer "+host.Token, info), http.StatusNoContent)
		// An outdated Agent's Reports are still accepted.
		expectStatus(t, h.postReport("Bearer "+host.Token, `{"time":1,"interval_ms":2000}`), http.StatusNoContent)
	}

	version, hosts := h.hostsWithInfo(session)
	if version != "v1.2.0" {
		t.Fatalf("dashboard_version = %q", version)
	}
	for _, host := range hosts {
		if want := host.Name == "old"; host.AgentOutdated != want {
			t.Errorf("%s: agent_outdated = %v, want %v", host.Name, host.AgentOutdated, want)
		}
	}
}
