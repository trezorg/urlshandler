package urlshandler

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var reqMalformedBody = []byte(`
1
2
3
`)
var reqBody = []byte(`http://1
http://2
http://3`)

type testClient struct {
	limiter limiter
}

func (c *testClient) makeHttpRequest(ctx context.Context, req *http.Request) (int64, error) {
	ticker := time.After(500 * time.Millisecond)
	select {
		case <- ctx.Done():
			return 0, ctx.Err()
		case <- ticker:
			return 0, nil
	}
}

func newTestClient(maxSimultaneousRequests int) *testClient {
	return &testClient{
		limiter: newLimiter(int32(maxSimultaneousRequests)),
	}
}

func TestParseMalformedRequestBody(t *testing.T) {
	reader := io.NopCloser(bytes.NewReader(reqMalformedBody))
	resp := parseRequestBody(reader)
	for _, res := range resp {
		assert.Error(t, res.err)
	}
}

func TestParseRequestBody(t *testing.T) {
	reader := io.NopCloser(bytes.NewReader(reqBody))
	resp := parseRequestBody(reader)
	assert.Len(t, resp, 3)
	for _, res := range resp {
		assert.NoError(t, res.err)
		assert.NotNil(t, res.url)
	}
}

func TestReadBodySize(t *testing.T) {
	testString := strings.Repeat("ok", 100000)
	reader := io.NopCloser(strings.NewReader(testString))
	size, err := readBodySize(reader)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(testString)), size)
}

func testSuccessResponse(t *testing.T, res *http.Response) {
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "chunked", res.Header.Get("Transfer-Encoding"))
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	urls := len(strings.Split(string(reqBody), "\n"))
	assert.Equal(t, strings.Repeat("0\n", urls), string(data))
}

func TestHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(reqBody))
	w := httptest.NewRecorder()
	handler := New(1, 5000*time.Millisecond, newTestClient(1))
	handler.ServeHTTP(w, req)
	res := w.Result()
	testSuccessResponse(t, res)
}

func TestHandlerTimeout(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(reqBody))
	w := httptest.NewRecorder()
	handler := New(1, 5*time.Millisecond, newTestClient(1))
	handler.ServeHTTP(w, req)
	res := w.Result()
	testSuccessResponse(t, res)
}

func TestHandlerLimiter(t *testing.T) {

	handler := New(1, 5000*time.Millisecond, newTestClient(1))
	var successResponses []*http.Response
	var throttledResponses []*http.Response

	wg := &sync.WaitGroup{}
	lock := &sync.Mutex{}

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(reqBody))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			res := w.Result()
			lock.Lock()
			if res.StatusCode == http.StatusTooManyRequests {
				throttledResponses = append(throttledResponses, res)
			} else {
				successResponses = append(successResponses, res)
			}
			lock.Unlock()
		}(wg)
	}

	wg.Wait()

	require.Len(t, successResponses, 1)
	require.Len(t, throttledResponses, 1)

	for _, res := range successResponses {
		testSuccessResponse(t, res)
	}

	for _, res := range throttledResponses {
		assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
	}

}
