package github

type Event struct {
	Name  string
	Label string
	Short string
}

var SupportedEvents = []Event{
	{Name: "branch_protection_configuration", Label: "Branch protection configuration", Short: "bpc"},
	{Name: "branch_protection_rule", Label: "Branch protection rules", Short: "bpr"},
	{Name: "check_run", Label: "Check runs", Short: "cr"},
	{Name: "check_suite", Label: "Check suites", Short: "cs"},
	{Name: "code_scanning_alert", Label: "Code scanning alerts", Short: "csa"},
	{Name: "commit_comment", Label: "Commit comments", Short: "cc"},
	{Name: "content_reference", Label: "Content references", Short: "ctr"},
	{Name: "create", Label: "Branches and tags created", Short: "crt"},
	{Name: "custom_property_values", Label: "Custom property values", Short: "cpv"},
	{Name: "delete", Label: "Branches and tags deleted", Short: "del"},
	{Name: "dependabot_alert", Label: "Dependabot alerts", Short: "da"},
	{Name: "deploy_key", Label: "Deploy keys", Short: "dk"},
	{Name: "deployment", Label: "Deployments", Short: "dep"},
	{Name: "deployment_status", Label: "Deployment statuses", Short: "ds"},
	{Name: "discussion", Label: "Discussions", Short: "dis"},
	{Name: "discussion_comment", Label: "Discussion comments", Short: "dc"},
	{Name: "fork", Label: "Forks", Short: "f"},
	{Name: "gollum", Label: "Wikis", Short: "g"},
	{Name: "issue_comment", Label: "Issue and PR comments", Short: "ic"},
	{Name: "issue_dependencies", Label: "Issue dependencies", Short: "idp"},
	{Name: "issues", Label: "Issues", Short: "i"},
	{Name: "label", Label: "Labels", Short: "lbl"},
	{Name: "member", Label: "Repository collaborators", Short: "m"},
	{Name: "merge_group", Label: "Merge queue groups", Short: "mg"},
	{Name: "meta", Label: "Webhook lifecycle", Short: "mt"},
	{Name: "milestone", Label: "Milestones", Short: "ms"},
	{Name: "package", Label: "Packages", Short: "pkg"},
	{Name: "page_build", Label: "GitHub Pages builds", Short: "pb"},
	{Name: "ping", Label: "Webhook pings", Short: "ping"},
	{Name: "project", Label: "Projects classic", Short: "pj"},
	{Name: "project_card", Label: "Project cards", Short: "pcd"},
	{Name: "project_column", Label: "Project columns", Short: "pco"},
	{Name: "public", Label: "Repository visibility", Short: "pub"},
	{Name: "pull_request", Label: "Pull requests", Short: "pr"},
	{Name: "pull_request_review", Label: "Pull request reviews", Short: "prr"},
	{Name: "pull_request_review_comment", Label: "PR review comments", Short: "prc"},
	{Name: "pull_request_review_thread", Label: "PR review threads", Short: "prt"},
	{Name: "pull_request_target", Label: "Pull request target", Short: "pt"},
	{Name: "push", Label: "Pushes", Short: "p"},
	{Name: "registry_package", Label: "Registry packages", Short: "rp"},
	{Name: "release", Label: "Releases", Short: "rel"},
	{Name: "repository", Label: "Repository changes", Short: "rep"},
	{Name: "repository_advisory", Label: "Repository advisories", Short: "rad"},
	{Name: "repository_import", Label: "Repository imports", Short: "ri"},
	{Name: "repository_ruleset", Label: "Repository rulesets", Short: "rr"},
	{Name: "repository_vulnerability_alert", Label: "Vulnerability alerts", Short: "rva"},
	{Name: "secret_scanning_alert", Label: "Secret scanning alerts", Short: "ssa"},
	{Name: "secret_scanning_alert_location", Label: "Secret scanning locations", Short: "ssl"},
	{Name: "secret_scanning_scan", Label: "Secret scanning scans", Short: "sss"},
	{Name: "security_and_analysis", Label: "Security and analysis", Short: "saa"},
	{Name: "star", Label: "Stars", Short: "s"},
	{Name: "status", Label: "Commit statuses", Short: "st"},
	{Name: "sub_issues", Label: "Sub-issues", Short: "sub"},
	{Name: "team_add", Label: "Team repository access", Short: "ta"},
	{Name: "user", Label: "Users", Short: "u"},
	{Name: "watch", Label: "Watches", Short: "w"},
	{Name: "workflow_job", Label: "Workflow jobs", Short: "wj"},
	{Name: "workflow_run", Label: "Workflow runs", Short: "wr"},
}

// EventPresets are named bundles of events offered as one-tap buttons in the
// repo settings menu. Keys are used as callback data, so keep them short.
var EventPresets = map[string]Preset{
	"ci": {
		Label:  "CI / builds",
		Events: []string{"check_run", "check_suite", "workflow_run", "workflow_job", "deployment", "deployment_status", "status"},
	},
	"issues": {
		Label:  "Issues",
		Events: []string{"issues", "issue_comment", "issue_dependencies", "label", "milestone"},
	},
	"prs": {
		Label:  "Pull requests",
		Events: []string{"pull_request", "pull_request_review", "pull_request_review_comment", "pull_request_review_thread", "pull_request_target", "status"},
	},
	"releases": {
		Label:  "Releases & deployments",
		Events: []string{"release", "deployment", "deployment_status", "push"},
	},
	"security": {
		Label:  "Security alerts",
		Events: []string{"secret_scanning_alert", "secret_scanning_alert_location", "dependabot_alert", "repository_advisory", "code_scanning_alert"},
	},
}

// Preset is a named bundle of GitHub webhook events.
type Preset struct {
	Label  string
	Events []string
}

// RepoHookForbiddenEvents are event types GitHub rejects when set on a
// repository webhook (they only apply to GitHub App or org-level hooks).
// Sending any of these in an EditHook call returns 422 Validation Failed.
var RepoHookForbiddenEvents = map[string]bool{
	"custom_property":               true,
	"deployment_protection_rule":    true,
	"deployment_review":             true,
	"github_app_authorization":      true,
	"installation":                  true,
	"installation_repositories":     true,
	"installation_target":           true,
	"marketplace_purchase":          true,
	"membership":                    true,
	"organization":                  true,
	"org_block":                     true,
	"personal_access_token_request": true,
	"projects_v2":                   true,
	"projects_v2_item":              true,
	"projects_v2_status_update":     true,
	"repository_dispatch":           true,
	"security_advisory":             true,
	"sponsorship":                   true,
	"team":                          true,
	"workflow_dispatch":             true,
}

// FilterRepoHookEvents removes events GitHub does not allow on repository
// webhooks, so saves never fail with 422 Validation Failed.
func FilterRepoHookEvents(events []string) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		if !RepoHookForbiddenEvents[e] {
			out = append(out, e)
		}
	}
	return out
}
