package urlshandler

import (
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type handler struct {
	limiter limiter
	timeout time.Duration
	client  client
}

type Handler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.processRequest(w, r)
}

func New(maxSimultaneousRequests int, timeout time.Duration, client client) Handler {
	return &handler{
		limiter: newLimiter(int32(maxSimultaneousRequests)),
		timeout: timeout,
		client:  client,
	}
}

func Default(w http.ResponseWriter, r *http.Request) Handler {
	return &handler{
		limiter: newLimiter(int32(100)),
		timeout: 10 * time.Second,
		client:  newClient(3),
	}
}

func (h *handler) processRequest(w http.ResponseWriter, r *http.Request) {
	defer h.limiter.release()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := h.limiter.take(); err != nil {
		text := err.Error()
		w.WriteHeader(http.StatusTooManyRequests)
		w.Header().Set("Retry-After", strconv.Itoa(int(h.timeout.Seconds())))
		w.Header().Set("Content-Length", strconv.Itoa(len(text)))
		io.WriteString(w, text)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	results := make(chan int64)
	wg := &sync.WaitGroup{}
	for _, u := range parseRequestBody(r.Body) {
		wg.Add(1)
		go func(w *sync.WaitGroup, u requestURL) {
			defer wg.Done()
			size := int64(0)
			if u.err != nil {
				results <- size
				return
			}
			req := &http.Request{
				URL:    u.url,
				Method: http.MethodGet,
			}
			size, err := h.client.makeHttpRequest(ctx, req)
			if err != nil {
				log.Printf("%v\n", err)
				size = 0
			}
			results <- size
		}(wg, u)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	for size := range results {
		io.WriteString(w, strconv.FormatInt(size, 10)+"\n")
	}
}
