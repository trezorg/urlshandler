package urlshandler

import (
	"fmt"
	"sync/atomic"
)

type limiter struct {
	max     int32
	counter int32
}

func newLimiter(max int32) limiter {
	return limiter{max: max}
}

type throttleError struct{
	limit int32
}

func (e *throttleError) Error() string {
	return fmt.Sprintf("Simultaneous requests limited to %d", e.limit)
}

type limiterError struct{
}

func (e *limiterError) Error() string {
	return fmt.Sprintf("Inconsistent limiter state")
}

func (l *limiter) take() error {
	if atomic.AddInt32(&l.counter, 1) > l.max {
		return &throttleError{l.max}
	}
	return nil
}

func (l *limiter) release() error {
	if atomic.AddInt32(&l.counter, -1) < 0 {
		return &limiterError{}
	}
	return nil
}
