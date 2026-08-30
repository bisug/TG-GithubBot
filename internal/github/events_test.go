package github

import (
	"strings"
	"testing"

	gh "github.com/google/go-github/v90/github"
)

func TestSupportedEventsCoverGitHubDocsAndGoGithubTypes(t *testing.T) {
	supported := make(map[string]Event, len(SupportedEvents))
	shorts := make(map[string]string, len(SupportedEvents))
	for _, event := range SupportedEvents {
		if event.Name == "" {
			t.Fatal("SupportedEvents contains empty event name")
		}
		if event.Label == "" {
			t.Fatalf("SupportedEvents[%q] has empty label", event.Name)
		}
		if event.Short == "" {
			t.Fatalf("SupportedEvents[%q] has empty short code", event.Name)
		}
		if prev, ok := supported[event.Name]; ok {
			t.Fatalf("duplicate supported event name %q: %#v and %#v", event.Name, prev, event)
		}
		if prev, ok := shorts[event.Short]; ok {
			t.Fatalf("duplicate supported event short code %q for %q and %q", event.Short, prev, event.Name)
		}
		supported[event.Name] = event
		shorts[event.Short] = event.Name
	}

	for _, eventName := range githubDocsWebhookEvents() {
		if _, ok := supported[eventName]; !ok {
			t.Fatalf("GitHub Docs webhook event %q is missing from SupportedEvents", eventName)
		}
	}

	for _, eventName := range gh.MessageTypes() {
		if _, ok := supported[eventName]; !ok {
			t.Fatalf("go-github webhook event %q is missing from SupportedEvents", eventName)
		}
	}
}

func TestParseWebhookEventFallsBackToGenericEvent(t *testing.T) {
	payload := []byte(`{
		"action":"created",
		"repository":{"full_name":"owner/repo","html_url":"https://github.com/owner/repo"},
		"sender":{"login":"octocat","html_url":"https://github.com/octocat"}
	}`)

	event, err := parseWebhookEvent("sub_issues", payload)
	if err != nil {
		t.Fatalf("parseWebhookEvent() error = %v", err)
	}

	generic, ok := event.(*GenericWebhookEvent)
	if !ok {
		t.Fatalf("parseWebhookEvent() = %T, want *GenericWebhookEvent", event)
	}
	if generic.EventType != "sub_issues" {
		t.Fatalf("EventType = %q, want sub_issues", generic.EventType)
	}
	if generic.Action != "created" {
		t.Fatalf("Action = %q, want created", generic.Action)
	}

	msg, markup := FormatGenericWebhookEvent(generic)
	for _, want := range []string{"Sub Issues Created", "owner/repo", "octocat"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("FormatGenericWebhookEvent() = %q, want substring %q", msg, want)
		}
	}
	if markup == nil {
		t.Fatal("FormatGenericWebhookEvent() returned nil markup for payload with GitHub URL")
	}
}

func TestParseWebhookEventUsesTypedGoGithubEvents(t *testing.T) {
	payload := []byte(`{"zen":"Keep it logically awesome.","hook_id":1}`)

	event, err := parseWebhookEvent("ping", payload)
	if err != nil {
		t.Fatalf("parseWebhookEvent() error = %v", err)
	}

	if _, ok := event.(*gh.PingEvent); !ok {
		t.Fatalf("parseWebhookEvent() = %T, want *github.PingEvent", event)
	}
}

func githubDocsWebhookEvents() []string {
	return []string{
		"branch_protection_configuration",
		"branch_protection_rule",
		"check_run",
		"check_suite",
		"code_scanning_alert",
		"commit_comment",
		"create",
		"custom_property",
		"custom_property_values",
		"delete",
		"dependabot_alert",
		"deploy_key",
		"deployment",
		"deployment_protection_rule",
		"deployment_review",
		"deployment_status",
		"discussion",
		"discussion_comment",
		"fork",
		"github_app_authorization",
		"gollum",
		"installation",
		"installation_repositories",
		"installation_target",
		"issue_comment",
		"issue_dependencies",
		"issues",
		"label",
		"marketplace_purchase",
		"member",
		"membership",
		"merge_group",
		"meta",
		"milestone",
		"organization",
		"org_block",
		"package",
		"page_build",
		"personal_access_token_request",
		"ping",
		"project",
		"project_card",
		"project_column",
		"projects_v2",
		"projects_v2_item",
		"projects_v2_status_update",
		"public",
		"pull_request",
		"pull_request_review",
		"pull_request_review_comment",
		"pull_request_review_thread",
		"push",
		"registry_package",
		"release",
		"repository",
		"repository_advisory",
		"repository_dispatch",
		"repository_import",
		"repository_ruleset",
		"repository_vulnerability_alert",
		"secret_scanning_alert",
		"secret_scanning_alert_location",
		"secret_scanning_scan",
		"security_advisory",
		"security_and_analysis",
		"sponsorship",
		"star",
		"status",
		"sub_issues",
		"team",
		"team_add",
		"watch",
		"workflow_dispatch",
		"workflow_job",
		"workflow_run",
	}
}
