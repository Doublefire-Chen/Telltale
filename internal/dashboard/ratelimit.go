package dashboard

import (
	"sync"
	"time"
)

// failureLimiter blocks a key after too many failures within a sliding window.
type failureLimiter struct {
	max    int
	window time.Duration

	mu       sync.Mutex
	failures map[string][]time.Time
}

func newFailureLimiter(max int, window time.Duration) *failureLimiter {
	return &failureLimiter{max: max, window: window, failures: make(map[string][]time.Time)}
}

// blocked reports whether key is currently blocked and, if so, how long until it is not.
func (l *failureLimiter) blocked(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.prune(key, now)
	if len(recent) < l.max {
		return false, 0
	}
	return true, recent[0].Add(l.window).Sub(now)
}

// fail records a failure for key at now.
func (l *failureLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.prune(key, now), now)
}

// reset forgets all failures for key.
func (l *failureLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops failures outside the window; the caller must hold l.mu.
func (l *failureLimiter) prune(key string, now time.Time) []time.Time {
	times := l.failures[key]
	i := 0
	for i < len(times) && !times[i].After(now.Add(-l.window)) {
		i++
	}
	times = times[i:]
	if len(times) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = times
	return times
}
