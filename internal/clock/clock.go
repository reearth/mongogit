package clock

import (
	"sync"
	"time"
)

var (
	mu sync.Mutex
	fn func() time.Time
)

func Now() time.Time {
	mu.Lock()
	defer mu.Unlock()
	if fn == nil {
		return time.Now()
	}
	return fn()
}

func Mock(t time.Time) func() {
	mu.Lock()
	defer mu.Unlock()
	fn = func() time.Time { return t }
	return func() {
		mu.Lock()
		defer mu.Unlock()
		fn = nil
	}
}
