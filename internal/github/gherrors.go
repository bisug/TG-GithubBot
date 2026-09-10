package github

import (
	"errors"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v90/github"
)

// IsInvalidTokenError reports whether err is a GitHub API response indicating
// the OAuth token is dead: HTTP 401, or a 403 whose message explicitly says
// "bad credentials". A bare 403 is ambiguous (it also covers legitimate
// permission denials and rate limits), so only an explicit credentials
// complaint clears the token.
func IsInvalidTokenError(err error) bool {
	var errResp *gh.ErrorResponse
	if !errors.As(err, &errResp) {
		return false
	}
	switch errResp.Response.StatusCode {
	case http.StatusUnauthorized:
		return true
	case http.StatusForbidden:
		return strings.Contains(strings.ToLower(errResp.Message), "bad credentials")
	default:
		return false
	}
}

// IsNotFoundError reports whether err is a GitHub 404 API response. GitHub
// returns 404 both when the resource truly does not exist and, deliberately,
// when the connected account lacks permission to see/manage it (e.g. editing
// a webhook on a repo the account is not an admin of).
func IsNotFoundError(err error) bool {
	var errResp *gh.ErrorResponse
	return errors.As(err, &errResp) && errResp.Response.StatusCode == http.StatusNotFound
}
