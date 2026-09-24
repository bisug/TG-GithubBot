package github

import (
	"strings"
	"testing"

	gh "github.com/google/go-github/v91/github"
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

func TestTitleTextPreservesUnicode(t *testing.T) {
	for input, want := range map[string]string{
		"opened":    "Opened",
		"élan_移动":   "Élan 移动",
		"Straße":    "Straße",
		"🚀launched": "🚀launched",
	} {
		if got := titleText(input); got != want {
			t.Errorf("titleText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatSecurityAndAnalysisMalformedPayload(t *testing.T) {
	msg, _ := FormatSecurityAndAnalysisEvent(&gh.SecurityAndAnalysisEvent{})
	if !strings.Contains(msg, "Security & Analysis Settings Updated") {
		t.Fatalf("malformed payload should still format safely, got %q", msg)
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

	if strings.Count(msg, "• <a href=") != 10 {
		t.Fatalf("FormatPushEvent listed %d commits, want 10", strings.Count(msg, "• <a href="))
	}
	if strings.Contains(msg, "slow body") {
		t.Fatalf("FormatPushEvent included commit body: %q", msg)
	}
	if !strings.Contains(msg, "+2 more commits not shown") {
		t.Fatalf("FormatPushEvent message = %q, want remaining commit count", msg)
	}
}

func TestFormatPushEventCommitLinkHrefAndText(t *testing.T) {
	// Regression: href and visible text were swapped, producing
	// <a href="0123456">https://github.com/.../commit/...</a>.
	msg, _ := FormatPushEvent(&gh.PushEvent{
		Ref: gh.Ptr("refs/heads/main"),
		Repo: &gh.PushEventRepository{
			FullName: gh.Ptr("owner/repo"),
			HTMLURL:  gh.Ptr("https://github.com/owner/repo"),
		},
		Commits: []*gh.HeadCommit{{
			ID:      gh.Ptr("0123456789abcdef"),
			Message: gh.Ptr("fix: thing"),
			Author:  &gh.CommitAuthor{Name: gh.Ptr("octocat")},
		}},
	})

	want := `<a href="https://github.com/owner/repo/commit/0123456789abcdef"><code>0123456</code></a>`
	if !strings.Contains(msg, want) {
		t.Fatalf("FormatPushEvent commit link = %q, want %q", msg, want)
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
func TestFormatCommitCommentEventCommitLinkHrefAndText(t *testing.T) {
	// Regression: href and visible text swapped, producing
	// <a href="0123456">https://github.com/...</a>, mirroring the push bug.
	msg, _ := FormatCommitCommentEvent(&gh.CommitCommentEvent{
		Action: gh.Ptr("created"),
		Repo: &gh.Repository{
			FullName: gh.Ptr("owner/repo"),
			HTMLURL:  gh.Ptr("https://github.com/owner/repo"),
		},
		Sender: &gh.User{Login: gh.Ptr("octocat")},
		Comment: &gh.RepositoryComment{
			CommitID: gh.Ptr("0123456789abcdef"),
			HTMLURL:  gh.Ptr("https://github.com/owner/repo/commit/0123456789abcdef"),
		},
	})
	want := `<a href="https://github.com/owner/repo/commit/0123456789abcdef"><code>0123456</code></a>`
	if !strings.Contains(msg, want) {
		t.Fatalf("FormatCommitCommentEvent commit link missing %q in:\n%s", want, msg)
	}
}

func TestFormatStatusEventCommitLinkHrefAndText(t *testing.T) {
	// Regression: same href/text swap in the status formatter.
	msg, _ := FormatStatusEvent(&gh.StatusEvent{
		State: gh.Ptr("success"),
		Commit: &gh.RepositoryCommit{
			SHA:     gh.Ptr("0123456789abcdef"),
			HTMLURL: gh.Ptr("https://github.com/owner/repo/commit/0123456789abcdef"),
			Commit:  &gh.Commit{Message: gh.Ptr("Build passed")},
		},
		Repo:    &gh.Repository{FullName: gh.Ptr("owner/repo"), HTMLURL: gh.Ptr("https://github.com/owner/repo")},
		Sender:  &gh.User{Login: gh.Ptr("octocat")},
		Context: gh.Ptr("ci"),
	})
	want := `<a href="https://github.com/owner/repo/commit/0123456789abcdef"><code>0123456</code></a>`
	if !strings.Contains(msg, want) {
		t.Fatalf("FormatStatusEvent commit link missing %q in:\n%s", want, msg)
	}
}

func TestFormatWorkflowRunEventOmitsStatusWhenNoConclusion(t *testing.T) {
	msg, _ := FormatWorkflowRunEvent(&gh.WorkflowRunEvent{
		Action:   gh.Ptr("completed"),
		Workflow: &gh.Workflow{Name: gh.Ptr("CI")},
		WorkflowRun: &gh.WorkflowRun{
			Status:  gh.Ptr("completed"),
			HTMLURL: gh.Ptr("https://github.com/owner/repo/actions/runs/1"),
		},
		Repo:   &gh.Repository{FullName: gh.Ptr("owner/repo")},
		Sender: &gh.User{Login: gh.Ptr("octocat")},
	})
	if strings.Contains(msg, "<b>Status:</b>") {
		t.Fatalf("FormatWorkflowRunEvent with empty conclusion should omit the Status line, got:\n%s", msg)
	}
	if !strings.Contains(msg, "<b>Workflow Run Completed in</b>") {
		t.Fatalf("FormatWorkflowRunEvent header = %q, want title-cased 'Completed'", msg)
	}
}

func TestFormatSponsorshipTierChangeHasNoPlaceholder(t *testing.T) {
	msg, _ := FormatSponsorshipEvent(&gh.SponsorshipEvent{
		Action: gh.Ptr("tier_changed"),
		Sender: &gh.User{Login: gh.Ptr("octocat"), HTMLURL: gh.Ptr("https://github.com/octocat")},
		Changes: &gh.SponsorshipChanges{
			Tier: &gh.SponsorshipTier{From: gh.Ptr("bronze")},
		},
	})
	if strings.Contains(msg, "new_tier") {
		t.Fatalf("FormatSponsorshipEvent rendered the literal 'new_tier' placeholder:\n%s", msg)
	}
	if !strings.Contains(msg, "<b>Tier Changed From:</b> <code>bronze</code>") {
		t.Fatalf("FormatSponsorshipEvent tier change missing, got:\n%s", msg)
	}
}

func TestFormatHeadersUseTitleCaseActionAndFieldLines(t *testing.T) {
	issueMsg, _ := FormatIssuesEvent(&gh.IssuesEvent{
		Action: gh.Ptr("opened"),
		Repo:   &gh.Repository{FullName: gh.Ptr("owner/repo"), HTMLURL: gh.Ptr("https://github.com/owner/repo")},
		Sender: &gh.User{Login: gh.Ptr("octocat")},
		Issue:  &gh.Issue{Title: gh.Ptr("A bug"), Number: gh.Ptr(1), HTMLURL: gh.Ptr("https://github.com/owner/repo/issues/1")},
	})
	if !strings.Contains(issueMsg, "📌 <b>Issue Opened #1</b>") {
		t.Fatalf("FormatIssuesEvent header = %q, want '📌 <b>Issue Opened #1</b>'", issueMsg)
	}

	milestoneMsg, _ := FormatMilestoneEvent(&gh.MilestoneEvent{
		Action: gh.Ptr("opened"),
		Repo:   &gh.Repository{FullName: gh.Ptr("owner/repo"), HTMLURL: gh.Ptr("https://github.com/owner/repo")},
		Sender: &gh.User{Login: gh.Ptr("octocat")},
	})
	if !strings.Contains(milestoneMsg, "🏁 <b>Milestone Opened</b>") {
		t.Fatalf("FormatMilestoneEvent header = %q, want '🏁 <b>Milestone Opened</b>'", milestoneMsg)
	}
}

func TestFormatDeploymentAndDeployKeyRepoLinks(t *testing.T) {
	wantLink := `<a href="https://github.com/owner/repo">owner/repo</a>`

	deployMsg, _ := FormatDeploymentEvent(&gh.DeploymentEvent{
		Repo: &gh.Repository{FullName: gh.Ptr("owner/repo"), Name: gh.Ptr("repo")},
	})
	if !strings.Contains(deployMsg, wantLink) {
		t.Fatalf("FormatDeploymentEvent repo link = %q, want full name link %q", deployMsg, wantLink)
	}

	keyMsg, _ := FormatDeployKeyEvent(&gh.DeployKeyEvent{
		Action: gh.Ptr("created"),
		Repo:   &gh.Repository{FullName: gh.Ptr("owner/repo"), Name: gh.Ptr("repo")},
	})
	if !strings.Contains(keyMsg, wantLink) {
		t.Fatalf("FormatDeployKeyEvent repo link = %q, want full name link %q", keyMsg, wantLink)
	}

	statusMsg, _ := FormatDeploymentStatusEvent(&gh.DeploymentStatusEvent{
		DeploymentStatus: &gh.DeploymentStatus{State: gh.Ptr("success")},
		Repo:             &gh.Repository{FullName: gh.Ptr("owner/repo"), Name: gh.Ptr("repo")},
	})
	if !strings.Contains(statusMsg, wantLink) {
		t.Fatalf("FormatDeploymentStatusEvent repo link = %q, want full name link %q", statusMsg, wantLink)
	}
}
