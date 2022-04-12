package urlshandler

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLimiterTake(t *testing.T) {
	l := newLimiter(5)
	for i := int32(0); i < l.max; i++ {
		assert.NoError(t, l.take())
	}
	assert.Equal(t, l.counter, l.max)
	assert.Error(t, l.take())
}

func TestLimiterRelease(t *testing.T) {
	l := newLimiter(5)
	assert.Error(t, l.release())
}

func TestLimiterTakeAndRelease(t *testing.T) {
	l := newLimiter(5)
	for i := int32(0); i < l.max; i++ {
		assert.NoError(t, l.take())
		assert.NoError(t, l.release())
	}
	assert.Equal(t, l.counter, int32(0))
}

func TestLimiterTakeAndReleaseConcurrent(t *testing.T) {
	l := newLimiter(5)
	wg := &sync.WaitGroup{}

	for i := int32(0); i < l.max; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			assert.NoError(t, l.take())
		}(wg)
	}

	wg.Wait()
	assert.Equal(t, l.counter, l.max)
}
