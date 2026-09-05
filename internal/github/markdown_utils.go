package github

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github-webhook/internal/bot/ui"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// Telegram HTML supports a small tag subset; these cover everything we emit.
var (
	// fenceRe matches fenced code blocks in markdown bodies.
	fenceRe = regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\n?(.*?)```")
	// inlineCodeRe matches `inline code` spans.
	inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")
	// mdLinkRe matches [text](url) links.
	mdLinkRe = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
	// boldStarRe matches **bold**, boldUsRe matches __bold__.
	boldStarRe = regexp.MustCompile(`\*\*([^\n]+?)\*\*`)
	boldUsRe   = regexp.MustCompile(`__([^\n]+?)__`)
	// italicRe matches *italic* and _italic_ not embedded in words.
	italicRe = regexp.MustCompile(`(^|[^\w*<>])[*_]([^*_\n]+?)[*_]([^\w*<>]|$)`)
	// strikeRe matches ~~strikethrough~~.
	strikeRe = regexp.MustCompile(`~~([^~\n]+?)~~`)
	// tagRe strips any remaining HTML tags from plain-text fallbacks.
	tagRe = regexp.MustCompile(`(?s)<[^>]*>`)
)

// EscapeHTML escapes text for Telegram's HTML parse mode. Only &, < and >
// are special in text content; html.EscapeString also covers quotes, which
// is safe (and required inside attribute values).
func EscapeHTML(text string) string {
	return html.EscapeString(text)
}

// EscapeHTMLURL escapes a URL for use inside an href attribute.
func EscapeHTMLURL(text string) string {
	return html.EscapeString(text)
}

// MarkdownToTelegramHTML converts a markdown body (GitHub issue/PR/release
// text) into the Telegram HTML subset. Unconvertible constructs degrade
// gracefully to plain escaped text.
func MarkdownToTelegramHTML(body string) string {
	var fences []string
	protected := fenceRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := fenceRe.FindStringSubmatch(m)
		fences = append(fences, "<pre>"+EscapeHTML(strings.Trim(sub[1], "\n"))+"</pre>")
		return fmt.Sprintf("\x00FENCE%d\x00", len(fences)-1)
	})

	// Escape everything else before adding markup.
	protected = EscapeHTML(protected)

	var codes []string
	protected = inlineCodeRe.ReplaceAllStringFunc(protected, func(m string) string {
		sub := inlineCodeRe.FindStringSubmatch(m)
		codes = append(codes, "<code>"+sub[1]+"</code>")
		return fmt.Sprintf("\x00CODE%d\x00", len(codes)-1)
	})

	protected = mdLinkRe.ReplaceAllString(protected, `<a href="$2">$1</a>`)
	protected = boldStarRe.ReplaceAllString(protected, "<b>$1</b>")
	protected = boldUsRe.ReplaceAllString(protected, "<b>$1</b>")
	protected = italicRe.ReplaceAllString(protected, "$1<i>$2</i>$3")
	protected = strikeRe.ReplaceAllString(protected, "<s>$1</s>")

	for i, c := range codes {
		protected = strings.Replace(protected, fmt.Sprintf("\x00CODE%d\x00", i), c, 1)
	}
	for i, f := range fences {
		protected = strings.Replace(protected, fmt.Sprintf("\x00FENCE%d\x00", i), f, 1)
	}
	return strings.TrimSpace(protected)
}

// FormatTextWithMarkdown renders a GitHub markdown body as Telegram HTML.
func FormatTextWithMarkdown(body string) string {
	if body == "" {
		return ""
	}
	return MarkdownToTelegramHTML(body)
}

// FormatReleaseBody renders a release notes body, collapsing long bodies
// behind a spoiler. Short bodies are shown as a blockquote.
func FormatReleaseBody(body string) string {
	htmlBody := FormatTextWithMarkdown(body)
	lines := strings.Split(htmlBody, "\n")
	const maxLines = 10
	const maxChars = 800
	isLong := len(lines) > maxLines || len(htmlBody) > maxChars

	if !isLong {
		return "<blockquote>" + htmlBody + "</blockquote>"
	}

	head := strings.Join(lines[:5], "\n")
	tail := strings.Join(lines[5:], "\n")
	return "<blockquote>" + head + "</blockquote><tg-spoiler>" + tail + "</tg-spoiler>"
}

// FormatRepo renders a repo name as a link to the repository.
func FormatRepo(repoFullName string) string {
	return fmt.Sprintf(`<a href="https://github.com/%s">%s</a>`,
		EscapeHTMLURL(repoFullName), EscapeHTML(repoFullName))
}

// FormatUser renders a login as a link to the profile.
func FormatUser(userLogin string) string {
	return fmt.Sprintf(`<a href="https://github.com/%s">%s</a>`,
		EscapeHTMLURL(userLogin), EscapeHTML(userLogin))
}

// StripTelegramHTML converts an HTML message to plain text for the
// no-format fallback send.
func StripTelegramHTML(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

func FormatMessageWithButton(message, buttonText, buttonURL string) (string, *gotgbot.InlineKeyboardMarkup) {
	if buttonText == "" || buttonURL == "" {
		return message, nil
	}
	return message, &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				ui.URL(buttonText, buttonURL,
					ui.WithStyle(ui.StylePrimary),
					ui.WithCustomEmojiEnv(ui.IconGitHub),
				),
			},
		},
	}
}

func ShortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}
