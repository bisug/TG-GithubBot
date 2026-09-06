package github

import (
	"strings"
	"testing"
)

func TestParseTypedWebhookEvents(t *testing.T) {
	cases := []struct {
		eventType string
		payload   string
		wantType  interface{}
		contains  []string
	}{
		{
			eventType: "issue_dependencies",
			payload: `{
				"action": "blocked_by_added",
				"blocked_issue": {"number": 2, "title": "Second", "html_url": "https://github.com/o/r/issues/2"},
				"blocking_issue": {"number": 1, "title": "First", "html_url": "https://github.com/o/r/issues/1"},
				"blocking_issue_repo": {"full_name": "o/r"},
				"repository": {"full_name": "o/r", "html_url": "https://github.com/o/r"},
				"sender": {"login": "octocat"}
			}`,
			wantType: &IssueDependenciesEvent{},
			contains: []string{"Issue Dependency Blocked By Added", "Blocked issue", "Blocking issue", "octocat"},
		},
		{
			eventType: "repository_advisory",
			payload: `{
				"action": "published",
				"repository_advisory": {"ghsa_id": "GHSA-xxxx", "summary": "Bad bug", "severity": "high", "cve_id": "CVE-2024-1234", "html_url": "https://github.com/o/r/security/advisories/GHSA-xxxx"},
				"repository": {"full_name": "o/r", "html_url": "https://github.com/o/r"},
				"sender": {"login": "octocat"}
			}`,
			wantType: &RepositoryAdvisoryEvent{},
			contains: []string{"Repository Advisory Published", "GHSA-xxxx", "High", "CVE-2024-1234"},
		},
		{
			eventType: "secret_scanning_scan",
			payload: `{
				"type": "backfill",
				"source": "push",
				"started_at": "2024-01-01T00:00:00Z",
				"completed_at": "2024-01-01T00:01:00Z",
				"secret_types": ["openai_api_key"],
				"repository": {"full_name": "o/r", "html_url": "https://github.com/o/r"}
			}`,
			wantType: &SecretScanningScanEvent{},
			contains: []string{"Secret Scanning Scan Completed", "backfill", "openai_api_key"},
		},
		{
			eventType: "sub_issues",
			payload: `{
				"action": "sub_issue_added",
				"parent_issue": {"number": 1, "title": "Parent", "html_url": "https://github.com/o/r/issues/1"},
				"sub_issue": {"number": 2, "title": "Child", "html_url": "https://github.com/o/r/issues/2"},
				"repository": {"full_name": "o/r", "html_url": "https://github.com/o/r"},
				"sender": {"login": "octocat"}
			}`,
			wantType: &SubIssuesEvent{},
			contains: []string{"Sub-issue Sub Issue Added", "Parent issue", "Sub-issue", "octocat"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			event, err := parseWebhookEvent(tc.eventType, []byte(tc.payload))
			if err != nil {
				t.Fatalf("parseWebhookEvent(%q) error: %v", tc.eventType, err)
			}
			if _, ok := event.(interface{ GetNumber() int }); ok {
				t.Fatalf("unexpected type %T", event)
			}

			var msg string
			switch e := event.(type) {
			case *IssueDependenciesEvent:
				msg, _ = FormatIssueDependenciesEvent(e)
			case *RepositoryAdvisoryEvent:
				msg, _ = FormatRepositoryAdvisoryEvent(e)
			case *SecretScanningScanEvent:
				msg, _ = FormatSecretScanningScanEvent(e)
			case *SubIssuesEvent:
				msg, _ = FormatSubIssuesEvent(e)
			default:
				t.Fatalf("parseWebhookEvent(%q) returned %T, want typed event", tc.eventType, event)
			}

			for _, want := range tc.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("%s message = %q, missing %q", tc.eventType, msg, want)
				}
			}
		})
	}
}

// TestParseTypedWebhookEventsReachableViaFormat ensures formatMessage dispatches
// the typed events instead of the generic card.
func TestParseTypedWebhookEventsReachableViaFormat(t *testing.T) {
	s := &WebhookServer{}
	event, err := parseWebhookEvent("sub_issues", []byte(`{
		"action": "sub_issue_added",
		"sub_issue": {"number": 2, "title": "Child", "html_url": "https://github.com/o/r/issues/2"},
		"repository": {"full_name": "o/r", "html_url": "https://github.com/o/r"}
	}`))
	if err != nil {
		t.Fatalf("parseWebhookEvent error: %v", err)
	}
	if _, ok := event.(*SubIssuesEvent); !ok {
		t.Fatalf("got %T, want *SubIssuesEvent", event)
	}

	msg, _ := s.formatMessage(event)
	if !strings.Contains(msg, "Sub-issue") {
		t.Fatalf("formatMessage = %q, want sub-issue formatting", msg)
	}

	if got := eventRepoFullName(event); got != "o/r" {
		t.Fatalf("eventRepoFullName = %q, want o/r", got)
	}
}
