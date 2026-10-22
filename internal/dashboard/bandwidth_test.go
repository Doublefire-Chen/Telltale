package dashboard_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// latestReport returns the first Host's latest Report as a generic map.
func (h *harness) latestReport(session *http.Cookie) map[string]any {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var body struct {
		Hosts []struct {
			Report map[string]any `json:"report"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		h.t.Fatal(err)
	}
	return body.Hosts[0].Report
}

func trafficReport(timeMS int64, rx, tx uint64) string {
	return fmt.Sprintf(`{"time":%d,"interval_ms":2000,"net_rx_bytes":%d,"net_tx_bytes":%d}`, timeMS, rx, tx)
}

func TestBandwidthFromConsecutiveReports(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token

	expectStatus(t, h.postReport(token, trafficReport(10_000, 1_000_000, 500_000)), http.StatusNoContent)
	first := h.latestReport(session)
	if _, ok := first["rx_rate"]; ok {
		t.Fatalf("first Report has a Bandwidth: %v", first)
	}

	// 2 seconds later: +4 MB in, +1 MB out.
	expectStatus(t, h.postReport(token, trafficReport(12_000, 5_000_000, 1_500_000)), http.StatusNoContent)
	second := h.latestReport(session)
	if second["rx_rate"] != 2_000_000.0 || second["tx_rate"] != 500_000.0 {
		t.Fatalf("Bandwidth rx=%v tx=%v, want 2000000 and 500000 bytes/s", second["rx_rate"], second["tx_rate"])
	}
	if second["net_rx_bytes"] != 5_000_000.0 || second["net_tx_bytes"] != 1_500_000.0 {
		t.Fatalf("Traffic rx=%v tx=%v, want the counters as sent", second["net_rx_bytes"], second["net_tx_bytes"])
	}
}

func TestBandwidthAfterReboot(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token

	expectStatus(t, h.postReport(token, trafficReport(10_000, 9_000_000, 9_000_000)), http.StatusNoContent)
	// The Host rebooted: counters restart near zero.
	expectStatus(t, h.postReport(token, trafficReport(70_000, 1_000, 2_000)), http.StatusNoContent)
	rep := h.latestReport(session)
	if _, ok := rep["rx_rate"]; ok {
		t.Fatalf("Bandwidth after a counter reset = %v, want none", rep["rx_rate"])
	}
	if rep["net_rx_bytes"] != 1_000.0 {
		t.Fatalf("Traffic after reboot = %v, want the new since-boot counter", rep["net_rx_bytes"])
	}

	// The next interval has a rate again.
	expectStatus(t, h.postReport(token, trafficReport(72_000, 3_000, 2_000)), http.StatusNoContent)
	rep = h.latestReport(session)
	if rep["rx_rate"] != 1_000.0 || rep["tx_rate"] != 0.0 {
		t.Fatalf("Bandwidth rx=%v tx=%v, want 1000 and 0", rep["rx_rate"], rep["tx_rate"])
	}
}

func TestBandwidthNeedsCountersAndForwardTime(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token

	expectStatus(t, h.postReport(token, trafficReport(10_000, 1_000, 1_000)), http.StatusNoContent)
	// Same sample time (or a clock that went backwards): no rate.
	expectStatus(t, h.postReport(token, trafficReport(10_000, 2_000, 2_000)), http.StatusNoContent)
	if _, ok := h.latestReport(session)["rx_rate"]; ok {
		t.Fatal("Bandwidth computed over a zero-length interval")
	}
	// No counters in this Report: no rate, and Traffic stays unknown.
	expectStatus(t, h.postReport(token, `{"time":12000,"interval_ms":2000}`), http.StatusNoContent)
	rep := h.latestReport(session)
	if _, ok := rep["rx_rate"]; ok {
		t.Fatal("Bandwidth computed without counters")
	}
	if _, ok := rep["net_rx_bytes"]; ok {
		t.Fatal("missing Traffic was filled in")
	}
}

func TestFullReportRoundTrips(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token

	body := `{"time":1,"interval_ms":2000,"cpu_percent":5,"memory_used":10,"swap_used":20,` +
		`"load1":0.5,"load5":0.25,"load15":0.125,"disk_used":30,"net_rx_bytes":40,"net_tx_bytes":50}`
	expectStatus(t, h.postReport(token, body), http.StatusNoContent)
	rep := h.latestReport(session)
	for field, want := range map[string]float64{
		"swap_used": 20, "load1": 0.5, "load5": 0.25, "load15": 0.125, "disk_used": 30, "net_rx_bytes": 40, "net_tx_bytes": 50,
	} {
		if rep[field] != want {
			t.Errorf("%s = %v, want %v", field, rep[field], want)
		}
	}
}
