package dashboard

import (
	"testing"
	"time"
)

func TestWindowLimiterSweepForgetsStaleKeys(t *testing.T) {
	l := newWindowLimiter(3, time.Minute)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, key := range []string{"2001:db8::1", "2001:db8::2", "2001:db8::3"} {
		l.add(key, now)
	}
	l.add("2001:db8::3", now.Add(50*time.Second))
	l.sweep(now.Add(70 * time.Second))
	if len(l.events) != 1 || len(l.events["2001:db8::3"]) != 1 {
		t.Fatalf("after sweep: %v, want only the recent event of 2001:db8::3", l.events)
	}
}
