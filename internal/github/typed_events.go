package github

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// This file adds typed payloads for webhook events that go-github v90 cannot
// parse (no structs in its event mapping). They are parsed in parseWebhookEvent
// before the generic fallback so they get rich formatting instead of the bare
// 📬 generic card. Payload shapes per GitHub webhook docs.

// genericEntity is defined in generic_webhook.go; these nil-safe accessors let
// formatters read optional fields without guarding every access.
func (g *genericEntity) GetFullName() string {
	if g == nil {
		return ""
	}
	return g.FullName
}
func (g *genericEntity) GetName() string {
	if g == nil {
		return ""
	}
	return g.Name
}
func (g *genericEntity) GetLogin() string {
	if g == nil {
		return ""
	}
	return g.Login
}
func (g *genericEntity) GetHTMLURL() string {
	if g == nil {
		return ""
	}
	return g.HTMLURL
}
func (g *genericEntity) GetURL() string {
	if g == nil {
		return ""
	}
	return g.URL
}

// IssueDependenciesEvent occurs when an issue is marked as blocked by or
// blocking another issue.
// Actions: blocked_by_added, blocked_by_removed, blocking_added, blocking_removed.
type IssueDependenciesEvent struct {
	Action            string         `json:"action,omitempty"`
	BlockedIssue      *genericIssue  `json:"blocked_issue,omitempty"`
	BlockingIssue     *genericIssue  `json:"blocking_issue,omitempty"`
	BlockingIssueRepo *genericEntity `json:"blocking_issue_repo,omitempty"`
	Repository        *genericEntity `json:"repository,omitempty"`
	Sender            *genericEntity `json:"sender,omitempty"`
}

// genericIssue is the subset of the GitHub issue object these payloads carry.
type genericIssue struct {
	ID      int64  `json:"id,omitempty"`
	Number  int    `json:"number,omitempty"`
	Title   string `json:"title,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
}

func (i *genericIssue) GetNumber() int     { return i.Number }
func (i *genericIssue) GetTitle() string   { return i.Title }
func (i *genericIssue) GetHTMLURL() string { return i.HTMLURL }

// RepositoryAdvisoryEvent occurs when a repository security advisory is
// published or reported.
// Actions: published, reported.
type RepositoryAdvisoryEvent struct {
	Action             string           `json:"action,omitempty"`
	RepositoryAdvisory *genericAdvisory `json:"repository_advisory,omitempty"`
	Repository         *genericEntity   `json:"repository,omitempty"`
	Sender             *genericEntity   `json:"sender,omitempty"`
}

// genericAdvisory is the subset of the repository advisory object used here.
type genericAdvisory struct {
	GHSAID   string `json:"ghsa_id,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Severity string `json:"severity,omitempty"`
	CVEID    string `json:"cve_id,omitempty"`
	HTMLURL  string `json:"html_url,omitempty"`
}

// SecretScanningScanEvent occurs when secret scanning completes a scan on a
// repository. It has no action field.
type SecretScanningScanEvent struct {
	Type               string         `json:"type,omitempty"`
	Source             string         `json:"source,omitempty"`
	StartedAt          string         `json:"started_at,omitempty"`
	CompletedAt        string         `json:"completed_at,omitempty"`
	SecretTypes        []string       `json:"secret_types,omitempty"`
	CustomPatternName  string         `json:"custom_pattern_name,omitempty"`
	CustomPatternScope string         `json:"custom_pattern_scope,omitempty"`
	Repository         *genericEntity `json:"repository,omitempty"`
	Sender             *genericEntity `json:"sender,omitempty"`
}

// SubIssuesEvent occurs when a parent/sub-issue relationship changes.
// Actions: parent_issue_added, parent_issue_removed, sub_issue_added, sub_issue_removed.
type SubIssuesEvent struct {
	Action          string         `json:"action,omitempty"`
	ParentIssue     *genericIssue  `json:"parent_issue,omitempty"`
	ParentIssueRepo *genericEntity `json:"parent_issue_repo,omitempty"`
	SubIssue        *genericIssue  `json:"sub_issue,omitempty"`
	Repository      *genericEntity `json:"repository,omitempty"`
	Sender          *genericEntity `json:"sender,omitempty"`
}

// parseTypedWebhookEvent parses the event types go-github v90 does not know
// into the local structs above. ok is false for any other event type.
func parseTypedWebhookEvent(eventType string, payload []byte) (interface{}, bool) {
	var (
		event interface{}
		err   error
	)
	switch eventType {
	case "issue_dependencies":
		event = &IssueDependenciesEvent{}
		err = json.Unmarshal(payload, event)
	case "repository_advisory":
		event = &RepositoryAdvisoryEvent{}
		err = json.Unmarshal(payload, event)
	case "secret_scanning_scan":
		event = &SecretScanningScanEvent{}
		err = json.Unmarshal(payload, event)
	case "sub_issues":
		event = &SubIssuesEvent{}
		err = json.Unmarshal(payload, event)
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	return event, true
}

func FormatIssueDependenciesEvent(e *IssueDependenciesEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.Action
	repo := firstNonEmpty(e.Repository.GetFullName(), e.Repository.GetName())
	sender := firstNonEmpty(e.Sender.GetLogin(), e.Sender.GetName())

	msg := fmt.Sprintf(
		"🔗 <b>Issue Dependency %s</b>\n\n"+
			"<b>Repository:</b> %s\n",
		EscapeHTML(titleText(action)),
		FormatRepo(repo),
	)

	var buttonURL string
	if e.BlockingIssue != nil {
		msg += fmt.Sprintf("<b>Blocking issue:</b> <a href=\"%s\">#%d %s</a>\n",
			EscapeHTMLURL(e.BlockingIssue.HTMLURL), e.BlockingIssue.Number, EscapeHTML(e.BlockingIssue.Title))
		buttonURL = e.BlockingIssue.HTMLURL
	}
	if e.BlockedIssue != nil {
		msg += fmt.Sprintf("<b>Blocked issue:</b> <a href=\"%s\">#%d %s</a>\n",
			EscapeHTMLURL(e.BlockedIssue.HTMLURL), e.BlockedIssue.Number, EscapeHTML(e.BlockedIssue.Title))
		if buttonURL == "" {
			buttonURL = e.BlockedIssue.HTMLURL
		}
	}
	if e.BlockingIssueRepo != nil && e.BlockingIssueRepo.GetFullName() != "" {
		msg += fmt.Sprintf("<b>Blocking issue repo:</b> %s\n", FormatRepo(e.BlockingIssueRepo.GetFullName()))
	}
	if sender != "" {
		msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(sender))
	}

	return FormatMessageWithButton(strings.TrimSpace(msg), "View Issue", buttonURL)
}

func FormatRepositoryAdvisoryEvent(e *RepositoryAdvisoryEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.Action
	repo := firstNonEmpty(e.Repository.GetFullName(), e.Repository.GetName())
	sender := firstNonEmpty(e.Sender.GetLogin(), e.Sender.GetName())
	adv := e.RepositoryAdvisory

	msg := fmt.Sprintf(
		"🛡️ <b>Repository Advisory %s</b>\n\n"+
			"<b>Repository:</b> %s\n",
		EscapeHTML(titleText(action)),
		FormatRepo(repo),
	)

	var buttonURL string
	if adv != nil {
		if adv.GHSAID != "" {
			msg += fmt.Sprintf("<b>Advisory:</b> <code>%s</code>\n", EscapeHTML(adv.GHSAID))
		}
		if adv.Summary != "" {
			msg += fmt.Sprintf("<b>Summary:</b> %s\n", EscapeHTML(adv.Summary))
		}
		if adv.Severity != "" {
			msg += fmt.Sprintf("<b>Severity:</b> %s\n", EscapeHTML(titleText(adv.Severity)))
		}
		if adv.CVEID != "" {
			msg += fmt.Sprintf("<b>CVE:</b> <code>%s</code>\n", EscapeHTML(adv.CVEID))
		}
		buttonURL = adv.HTMLURL
	}
	if sender != "" {
		msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(sender))
	}

	return FormatMessageWithButton(strings.TrimSpace(msg), "View Advisory", buttonURL)
}

func FormatSecretScanningScanEvent(e *SecretScanningScanEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := firstNonEmpty(e.Repository.GetFullName(), e.Repository.GetName())
	sender := firstNonEmpty(e.Sender.GetLogin(), e.Sender.GetName())

	msg := fmt.Sprintf(
		"🔍 <b>Secret Scanning Scan Completed</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Type:</b> <code>%s</code>\n"+
			"<b>Source:</b> <code>%s</code>\n",
		FormatRepo(repo),
		EscapeHTML(e.Type),
		EscapeHTML(e.Source),
	)

	if e.CustomPatternName != "" {
		msg += fmt.Sprintf("<b>Custom pattern:</b> <code>%s</code>\n", EscapeHTML(e.CustomPatternName))
		if e.CustomPatternScope != "" {
			msg += fmt.Sprintf("<b>Pattern scope:</b> <code>%s</code>\n", EscapeHTML(e.CustomPatternScope))
		}
	}
	if len(e.SecretTypes) > 0 {
		types := make([]string, 0, len(e.SecretTypes))
		for _, st := range e.SecretTypes {
			types = append(types, EscapeHTML(st))
		}
		msg += fmt.Sprintf("<b>Secret types:</b> %s\n", strings.Join(types, ", "))
	}
	if sender != "" {
		msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(sender))
	}

	var buttonURL string
	if e.Repository != nil {
		buttonURL = firstNonEmpty(e.Repository.GetHTMLURL(), e.Repository.GetURL())
	}
	return FormatMessageWithButton(strings.TrimSpace(msg), "View Repository", buttonURL)
}

func FormatSubIssuesEvent(e *SubIssuesEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.Action
	repo := firstNonEmpty(e.Repository.GetFullName(), e.Repository.GetName())
	sender := firstNonEmpty(e.Sender.GetLogin(), e.Sender.GetName())

	msg := fmt.Sprintf(
		"🧩 <b>Sub-issue %s</b>\n\n"+
			"<b>Repository:</b> %s\n",
		EscapeHTML(titleText(action)),
		FormatRepo(repo),
	)

	var buttonURL string
	if e.ParentIssue != nil {
		msg += fmt.Sprintf("<b>Parent issue:</b> <a href=\"%s\">#%d %s</a>\n",
			EscapeHTMLURL(e.ParentIssue.HTMLURL), e.ParentIssue.Number, EscapeHTML(e.ParentIssue.Title))
		buttonURL = e.ParentIssue.HTMLURL
	}
	if e.SubIssue != nil {
		msg += fmt.Sprintf("<b>Sub-issue:</b> <a href=\"%s\">#%d %s</a>\n",
			EscapeHTMLURL(e.SubIssue.HTMLURL), e.SubIssue.Number, EscapeHTML(e.SubIssue.Title))
		if buttonURL == "" {
			buttonURL = e.SubIssue.HTMLURL
		}
	}
	if e.ParentIssueRepo != nil && e.ParentIssueRepo.GetFullName() != "" {
		msg += fmt.Sprintf("<b>Parent issue repo:</b> %s\n", FormatRepo(e.ParentIssueRepo.GetFullName()))
	}
	if sender != "" {
		msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(sender))
	}

	return FormatMessageWithButton(strings.TrimSpace(msg), "View Issue", buttonURL)
}
