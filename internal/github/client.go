package github

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v85/github"
	"golang.org/x/oauth2"
)

// clientIdleTTL bounds how long an unused authenticated client is kept. The cache is
// keyed by access token and users re-authenticate over time, so without a TTL it would
// grow without bound for a long-running process.
const clientIdleTTL = 30 * time.Minute

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
// Clients are cached per access token to reuse the underlying TCP connection and avoid
// rebuilding the oauth2 transport on every call.
func (f *ClientFactory) GetUserClient(ctx context.Context, accessToken string) *github.Client {
	if v, ok := f.clients.Load(accessToken); ok {
		cc := v.(*cachedClient)
		cc.lastUsed.Store(time.Now().UnixNano())
		return cc.client
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	c := github.NewClient(oauth2.NewClient(ctx, ts))
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
