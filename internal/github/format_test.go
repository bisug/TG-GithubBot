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

	if strings.Count(msg, "- <a href=") != 10 {
		t.Fatalf("FormatPushEvent listed %d commits, want 10", strings.Count(msg, "- <a href="))
	}
	if strings.Contains(msg, "slow body") {
		t.Fatalf("FormatPushEvent included commit body: %q", msg)
	}
	if !strings.Contains(msg, "+2 more commits not shown") {
		t.Fatalf("FormatPushEvent message = %q, want remaining commit count", msg)
	}
}

func TestStripTelegramHTMLRemovesTagsAndDecodesEntities(t *testing.T) {
	got := StripTelegramHTML("<b>Title</b> &amp; <a href=\"https://github.com/owner/repo\">repo</a>")

	if strings.ContainsAny(got, "<>") {
		t.Fatalf("StripTelegramHTML() = %q, want HTML tags removed", got)
	}
	if !strings.Contains(got, "Title & repo") {
		t.Fatalf("StripTelegramHTML() = %q, want entities decoded and text preserved", got)
	}
}

func TestMarkdownToTelegramHTML(t *testing.T) {
	got := MarkdownToTelegramHTML("**bold** and *italic* with `code` and [link](https://example.com)\n\n```go\nfmt.Println(\"hi <>&\")\n```")

	for _, want := range []string{
		"<b>bold</b>",
		"<i>italic</i>",
		"<code>code</code>",
		`<a href="https://example.com">link</a>`,
		"<pre>",
		"fmt.Println(&#34;hi &lt;&gt;&amp;&#34;)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("MarkdownToTelegramHTML() = %q, want %q", got, want)
		}
	}
}

func TestFormatReleaseBodyShortUsesBlockquote(t *testing.T) {
	got := FormatReleaseBody("Short notes")
	if !strings.HasPrefix(got, "<blockquote>") || !strings.HasSuffix(got, "</blockquote>") {
		t.Fatalf("FormatReleaseBody() = %q, want blockquote wrapping", got)
	}
}

func TestFilterRepoHookEvents(t *testing.T) {
	got := FilterRepoHookEvents([]string{"push", "organization", "workflow_run", "security_advisory"})
	if len(got) != 2 || got[0] != "push" || got[1] != "workflow_run" {
		t.Fatalf("FilterRepoHookEvents() = %v, want [push workflow_run]", got)
	}
}

func TestSupportedEventsAndPresetsAreRepoHookAllowed(t *testing.T) {
	for _, e := range SupportedEvents {
		if RepoHookForbiddenEvents[e.Name] {
			t.Errorf("SupportedEvents contains repo-forbidden event %q", e.Name)
		}
	}
	for name, p := range EventPresets {
		for _, e := range p.Events {
			if RepoHookForbiddenEvents[e] {
				t.Errorf("preset %q contains repo-forbidden event %q", name, e)
			}
		}
	}
}
