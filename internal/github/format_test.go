package github

import (
	"strings"
	"testing"

	gh "github.com/google/go-github/v90/github"
)

func TestFormatPushEventIncludesZeroCommitBranchChanges(t *testing.T) {
	msg, markup := FormatPushEvent(&gh.PushEvent{
		Ref:     gh.Ptr("refs/heads/feature/test"),
		Created: gh.Ptr(true),
		Repo: &gh.PushEventRepository{
			FullName: gh.Ptr("owner/repo"),
			HTMLURL:  gh.Ptr("https://github.com/owner/repo"),
		},
	})

	if msg == "" {
		t.Fatal("FormatPushEvent returned empty message for zero-commit branch creation")
	}
	if !strings.Contains(msg, "New branch created") {
		t.Fatalf("FormatPushEvent message = %q, want branch creation text", msg)
	}
	if markup == nil {
		t.Fatal("FormatPushEvent returned nil markup for repository link")
	}
}

func TestFormatPushEventCapsCommitListAndUsesFirstLine(t *testing.T) {
	commits := make([]*gh.HeadCommit, 12)
	for i := range commits {
		commits[i] = &gh.HeadCommit{
			ID:      gh.Ptr("0123456789abcdef"),
			Message: gh.Ptr("first line\n\nslow body that should not be included"),
			Author:  &gh.CommitAuthor{Name: gh.Ptr("octocat")},
		}
	}

	msg, _ := FormatPushEvent(&gh.PushEvent{
		Ref:     gh.Ptr("refs/heads/main"),
		Compare: gh.Ptr("https://github.com/owner/repo/compare/a...b"),
		Repo: &gh.PushEventRepository{
			FullName: gh.Ptr("owner/repo"),
			HTMLURL:  gh.Ptr("https://github.com/owner/repo"),
		},
		Commits: commits,
	})

	if strings.Count(msg, "\\- [0123456]") != 10 {
		t.Fatalf("FormatPushEvent listed %d commits, want 10", strings.Count(msg, "\\- [0123456]"))
	}
	if strings.Contains(msg, "slow body") {
		t.Fatalf("FormatPushEvent included commit body: %q", msg)
	}
	if !strings.Contains(msg, "\\+2 more commits not shown") {
		t.Fatalf("FormatPushEvent message = %q, want remaining commit count", msg)
	}
}

func TestPlainTextForTelegramRemovesMarkdownControlCharacters(t *testing.T) {
	got := plainTextForTelegram("*Title* [repo](https://github.com/owner/repo) \\_escaped\\_")

	if strings.ContainsAny(got, "*_`~\\") {
		t.Fatalf("plainTextForTelegram() = %q, want Markdown control characters removed", got)
	}
	if !strings.Contains(got, "repo (https://github.com/owner/repo)") {
		t.Fatalf("plainTextForTelegram() = %q, want link text preserved", got)
	}
}
