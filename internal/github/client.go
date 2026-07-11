package github

import (
	"context"
	"sync"

	"github.com/google/go-github/v85/github"
	"golang.org/x/oauth2"
)

type ClientFactory struct {
	clients sync.Map // accessToken -> *github.Client
}

func NewClientFactory() *ClientFactory {
	return &ClientFactory{}
}

// GetUserClient returns a GitHub client authenticated as a specific User (via OAuth token).
// Clients are cached per access token to reuse the underlying TCP connection and avoid
// rebuilding the oauth2 transport on every call.
func (f *ClientFactory) GetUserClient(ctx context.Context, accessToken string) *github.Client {
	if c, ok := f.clients.Load(accessToken); ok {
		return c.(*github.Client)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(ctx, ts)
	c := github.NewClient(tc)
	f.clients.Store(accessToken, c)
	return c
}
