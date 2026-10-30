package github

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/google/go-github/v92/github"
	"golang.org/x/oauth2"
)

type ClientFactory struct {
	httpClient *http.Client
}

func NewClientFactory() *ClientFactory {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &ClientFactory{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   20 * time.Second,
		},
	}
}

// GetUserClient returns a GitHub client authenticated as a specific User (via OAuth token)
func (f *ClientFactory) GetUserClient(ctx context.Context, accessToken string) (*github.Client, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, f.httpClient)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(github.WithHTTPClient(tc))
}
