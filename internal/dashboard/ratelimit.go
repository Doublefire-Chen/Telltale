package dashboard

import (
	"sync"
	"time"
)

// windowLimiter blocks a key after too many events within a sliding window.
type windowLimiter struct {
	max    int
	window time.Duration

	mu     sync.Mutex
	events map[string][]time.Time
}

func newWindowLimiter(max int, window time.Duration) *windowLimiter {
	return &windowLimiter{max: max, window: window, events: make(map[string][]time.Time)}
}

// blocked reports whether key is currently blocked and, if so, how long until it is not.
func (l *windowLimiter) blocked(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.prune(key, now)
	if len(recent) < l.max {
		return false, 0
	}
	return true, recent[0].Add(l.window).Sub(now)
}

// add records an event for key at now.
func (l *windowLimiter) add(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events[key] = append(l.prune(key, now), now)
}

// reset forgets all events for key.
func (l *windowLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.events, key)
}

// sweep forgets keys with no events left in the window, so keys that are
// never seen again (an attacker's many addresses, say) do not pile up.
func (l *windowLimiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.events {
		l.prune(key, now)
	}
}

// prune drops events outside the window; the caller must hold l.mu.
func (l *windowLimiter) prune(key string, now time.Time) []time.Time {
	times := l.events[key]
	i := 0
	for i < len(times) && !times[i].After(now.Add(-l.window)) {
		i++
	}
	times = times[i:]
	if len(times) == 0 {
		delete(l.events, key)
		return nil
	}
	l.events[key] = times
	return times
}
