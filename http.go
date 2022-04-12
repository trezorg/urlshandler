package urlshandler

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func isRecoverableError(response *http.Response) bool {
	if response.StatusCode >= 500 {
		return true
	}
	return false
}

func readBodySize(resp io.ReadCloser) (int64, error) {
	defer resp.Close()
	length := int64(0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Read(buf)
		length += int64(n)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}
	}
	return length, nil
}

func retryWait(max, min int) {
	time.Sleep(time.Second * time.Duration(rand.Intn(max-min+1)+min))
}

func prepareClient() *http.Client {
	netTransport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
	}

	client := &http.Client{
		Transport: netTransport,
	}
	return client
}

type httpClient struct {
	maxRecoverAttempts int
	*http.Client
}

type client interface {
	makeHttpRequest(ctx context.Context, req *http.Request) (int64, error)
}

func newClient(maxAttempts int) *httpClient {
	return &httpClient{Client: prepareClient()}
}

func (c *httpClient) makeRecovarableHttpRequest(ctx context.Context, req *http.Request, attempts int) (int64, error) {
	req = req.WithContext(ctx)
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	if isRecoverableError(resp) {
		if attempts >= c.maxRecoverAttempts {
			return 0, nil
		}
		attempts += 1
		log.Printf("Recoverable error: %v\n. Attempts: %d", err, attempts)
		retryWait(3, 1)
		return c.makeRecovarableHttpRequest(ctx, req, attempts)
	}
	if resp.ContentLength != -1 {
		return resp.ContentLength, nil
	}
	return readBodySize(resp.Body)
}

func (client *httpClient) makeHttpRequest(ctx context.Context, req *http.Request) (int64, error) {
	return client.makeRecovarableHttpRequest(ctx, req, 0)
}

type requestURL struct {
	url *url.URL
	err error
}

func parseURL(u string) requestURL {
	uri, err := url.ParseRequestURI(u)
	if err != nil {
		return requestURL{url: uri, err: err}
	}
	if uri.Scheme == "" {
		uri.Scheme = "https"
	}
	if uri.Path == "" {
		uri.Path = "/"
	}
	return requestURL{url: uri}
}

func parseRequestBody(req io.ReadCloser) []requestURL {
	defer req.Close()
	scanner := bufio.NewScanner(req)
	var res []requestURL
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		res = append(res, parseURL(line))
	}
	return res
}
