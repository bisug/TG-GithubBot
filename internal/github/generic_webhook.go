package github

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	gh "github.com/google/go-github/v90/github"
)

type GenericWebhookEvent struct {
	EventType    string
	Action       string         `json:"action,omitempty"`
	Repository   *genericEntity `json:"repository,omitempty"`
	Organization *genericEntity `json:"organization,omitempty"`
	Sender       *genericEntity `json:"sender,omitempty"`
}

type genericEntity struct {
	FullName string `json:"full_name,omitempty"`
	Name     string `json:"name,omitempty"`
	Login    string `json:"login,omitempty"`
	HTMLURL  string `json:"html_url,omitempty"`
	URL      string `json:"url,omitempty"`
}

func parseWebhookEvent(eventType string, payload []byte) (interface{}, error) {
	event, err := gh.ParseWebHook(eventType, payload)
	if err == nil {
		return event, nil
	}

	generic, genericErr := parseGenericWebhookEvent(eventType, payload)
	if genericErr != nil {
		return nil, fmt.Errorf("%w; generic parse failed: %v", err, genericErr)
	}

	return generic, nil
}

func parseGenericWebhookEvent(eventType string, payload []byte) (*GenericWebhookEvent, error) {
	if strings.TrimSpace(eventType) == "" {
		return nil, fmt.Errorf("missing event type")
	}

	var event GenericWebhookEvent
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
	}
	event.EventType = eventType
	return &event, nil
}

func FormatGenericWebhookEvent(event *GenericWebhookEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if event == nil {
		return "", nil
	}

	title := titleText(event.EventType)
	if event.Action != "" {
		title += " " + titleText(event.Action)
	}

	msg := fmt.Sprintf("📬 <b>%s</b>\n\n", EscapeHTML(title))
	var buttonURL string

	if repo := event.Repository; repo != nil {
		repoName := firstNonEmpty(repo.FullName, repo.Name)
		if repoName != "" {
			msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(repoName))
		}
		buttonURL = firstNonEmpty(repo.HTMLURL, buttonURL)
	}

	if org := event.Organization; org != nil {
		orgName := firstNonEmpty(org.Login, org.Name)
		if orgName != "" {
			msg += fmt.Sprintf("<b>Organization:</b> %s\n", FormatUser(orgName))
		}
		buttonURL = firstNonEmpty(buttonURL, org.HTMLURL)
	}

	if sender := event.Sender; sender != nil {
		senderName := firstNonEmpty(sender.Login, sender.Name)
		if senderName != "" {
			msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(senderName))
		}
		buttonURL = firstNonEmpty(buttonURL, sender.HTMLURL)
	}

	if event.Action != "" {
		msg += fmt.Sprintf("<b>Action:</b> <code>%s</code>", EscapeHTML(event.Action))
	}

	return FormatMessageWithButton(strings.TrimSpace(msg), "View on GitHub", buttonURL)
}
