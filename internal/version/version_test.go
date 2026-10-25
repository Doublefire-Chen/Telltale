package version

import (
	"maps"
	"testing"
)

func TestAgentHashes(t *testing.T) {
	a, b := "7d28c965155e5afbe898b6aa00589e4b60374f6e1b33d6a37077f5acb52e6f06", "c338c31f6a347020eb4c969c5a1e9b3ce84c145464167f38fd78e146aac688fc"
	got := AgentHashes("linux-amd64=" + a + ",darwin-arm64=" + b + ",bad=xyz,=" + a + ",windows-amd64=" + a[:63] + "G")
	if want := map[string]string{"linux-amd64": a, "darwin-arm64": b}; !maps.Equal(got, want) {
		t.Fatalf("AgentHashes = %v, want %v", got, want)
	}
	if got := AgentHashes(""); len(got) != 0 {
		t.Fatalf("AgentHashes(\"\") = %v", got)
	}
}

func TestOlder(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.3", "v1.3.0", true},
		{"v1.9.9", "v2.0.0", true},
		{"1.2.3", "v1.10.0", true},
		{"v1.2.4", "v1.2.3", false},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3-rc.1", "v1.2.3", false},
		{"dev", "v1.0.0", false},
		{"v1.0.0", "dev", false},
		{"", "v1.0.0", false},
	} {
		if got := Older(tc.a, tc.b); got != tc.want {
			t.Errorf("Older(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
