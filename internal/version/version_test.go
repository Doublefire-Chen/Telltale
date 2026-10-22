package version

import "testing"

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
