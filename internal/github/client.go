package github

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v90/github"
)

// clientIdleTTL bounds how long an unused authenticated client is kept. The cache is
// keyed by access token and users re-authenticate over time, so without a TTL it would
// grow without bound for a long-running process.
const clientIdleTTL = 30 * time.Minute

// ghHTTPTimeout bounds every GitHub API call made through cached clients. Without
// it a hung connection parks the calling handler goroutine forever.
const ghHTTPTimeout = 30 * time.Second

type cachedClient struct {
	client   *github.Client
	lastUsed atomic.Int64 // unix nanos; atomic because GetUserClient is called concurrently
}

type ClientFactory struct {
	clients sync.Map // accessToken -> *cachedClient
}

func NewClientFactory() *ClientFactory {
	return &ClientFactory{}
}

// GetUserClient returns a GitHub client authenticated as a specific User (via OAuth token).
// Clients are cached per access token to reuse the underlying TCP connections.
func (f *ClientFactory) GetUserClient(_ context.Context, accessToken string) *github.Client {
	if v, ok := f.clients.Load(accessToken); ok {
		cc := v.(*cachedClient)
		cc.lastUsed.Store(time.Now().UnixNano())
		return cc.client
	}

	c, err := github.NewClient(github.WithAuthToken(accessToken), github.WithHTTPClient(&http.Client{Timeout: ghHTTPTimeout}))
	if err != nil {
		// WithAuthToken only fails on an empty token, which callers never pass.
		// ponytail: cache a best-effort client rather than complicating the
		// factory signature; upgrade path is returning an error from GetUserClient.
		c, _ = github.NewClient(github.WithHTTPClient(&http.Client{Timeout: ghHTTPTimeout}))
	}
	cc := &cachedClient{client: c}
	cc.lastUsed.Store(time.Now().UnixNano())
	f.clients.Store(accessToken, cc)
	return c
}

// Cleanup drops clients that have not been used within clientIdleTTL. Safe to call
// periodically; in-flight requests on an evicted client are unaffected.
func (f *ClientFactory) Cleanup() {
	now := time.Now().UnixNano()
	f.clients.Range(func(key, value any) bool {
		if now-value.(*cachedClient).lastUsed.Load() > int64(clientIdleTTL) {
			f.clients.Delete(key)
		}
		return true
	})
}
