package github

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github-webhook/internal/bot/ui"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// Telegram HTML supports a small tag subset; these cover everything we emit.
var (
	// fenceBacktickRe matches ```lang code blocks.
	fenceBacktickRe = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)[ \t]*\n?(.*?)```")
	// fenceTildeRe matches ~~~lang code blocks.
	fenceTildeRe = regexp.MustCompile("(?s)~~~([a-zA-Z0-9_+-]*)[ \t]*\n?(.*?)~~~")
	// inlineCodeRe matches `inline code` spans.
	inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")
	// Header regex: # Header through ###### Header at line start.
	headerRe = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+)$`)
	// Task lists: - [x] or - [ ] at start of line.
	taskDoneRe = regexp.MustCompile(`(?m)^([ \t]*)[-*+][ \t]+\[[xX]\][ \t]+`)
	taskTodoRe = regexp.MustCompile(`(?m)^([ \t]*)[-*+][ \t]+\[[ \t]\][ \t]+`)
	// Bullet lists: - item, * item, + item at start of line.
	bulletRe = regexp.MustCompile(`(?m)^([ \t]*)[-*+][ \t]+`)
	// Horizontal rules: ---, ***, ___ on their own line.
	hrRe = regexp.MustCompile(`(?m)^[ \t]*(?:---|\*\*\*|___)[ \t]*$`)
	// Bold + italic (3 asterisks or 3 underscores).
	boldItalicStarRe = regexp.MustCompile(`\*\*\*([^*\n]+?)\*\*\*`)
	boldItalicUsRe   = regexp.MustCompile(`___([^_\n]+?)___`)
	// Bold (2 asterisks or 2 underscores).
	boldStarRe = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	boldUsRe   = regexp.MustCompile(`__([^_\n]+?)__`)
	// Italic with separate delimiter pairs (no cross-matching between * and _).
	// Require non-whitespace immediately inside delimiters.
	italicStarRe = regexp.MustCompile(`(^|[^\w*])\*([^*\s\n](?:[^*\n]*?[^*\s\n])?)\*([^\w*]|$)`)
	italicUsRe   = regexp.MustCompile(`(^|[^\w_])_([^_\s\n](?:[^_\n]*?[^_\s\n])?)_([^\w_]|$)`)
	// Strikethrough: ~~strike~~.
	strikeRe = regexp.MustCompile(`~~([^~\n]+?)~~`)
	// Consecutive blank lines: collapse 3+ newlines into 2.
	consecutiveNewlinesRe = regexp.MustCompile(`\n{3,}`)
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
	if strings.TrimSpace(body) == "" {
		return ""
	}

	// -------------------------------------------------------------------------
	// Phase 1: Code block protection (tokens avoid interference from markdown passes)
	// -------------------------------------------------------------------------
	// Choose a token prefix absent from the input so attacker-controlled NUL/text
	// cannot imitate a placeholder and have generated HTML inserted into it.
	tokenPrefix := "\x00TG"
	for strings.Contains(body, tokenPrefix) {
		tokenPrefix += "_"
	}
	var fences []string
	extractFence := func(lang, code string) string {
		cleanCode := EscapeHTML(strings.Trim(code, "\r\n"))
		lang = strings.TrimSpace(lang)
		var block string
		if lang != "" {
			block = fmt.Sprintf(`<pre><code class="language-%s">%s</code></pre>`, EscapeHTML(lang), cleanCode)
		} else {
			block = "<pre>" + cleanCode + "</pre>"
		}
		fences = append(fences, block)
		return fmt.Sprintf("%sFENCE%d\x00", tokenPrefix, len(fences)-1)
	}

	protected := fenceBacktickRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := fenceBacktickRe.FindStringSubmatch(m)
		return extractFence(sub[1], sub[2])
	})
	protected = fenceTildeRe.ReplaceAllStringFunc(protected, func(m string) string {
		sub := fenceTildeRe.FindStringSubmatch(m)
		return extractFence(sub[1], sub[2])
	})

	var codes []string
	protected = inlineCodeRe.ReplaceAllStringFunc(protected, func(m string) string {
		sub := inlineCodeRe.FindStringSubmatch(m)
		codes = append(codes, "<code>"+EscapeHTML(sub[1])+"</code>")
		return fmt.Sprintf("%sCODE%d\x00", tokenPrefix, len(codes)-1)
	})

	// -------------------------------------------------------------------------
	// Phase 2: HTML Escaping for remaining non-code text
	// -------------------------------------------------------------------------
	protected = EscapeHTML(protected)

	// -------------------------------------------------------------------------
	// Phase 3: Block-level markdown structures
	// -------------------------------------------------------------------------
	// Task lists before regular bullet lists
	protected = taskDoneRe.ReplaceAllString(protected, "$1• ✅ ")
	protected = taskTodoRe.ReplaceAllString(protected, "$1• ◻️ ")
	// Bullet lists: converts -, *, + at line start to bullet symbol •
	// (eliminates asterisk bullet points from triggering italic matching)
	protected = bulletRe.ReplaceAllString(protected, "$1• ")
	// Headers: # Header -> <b>Header</b>
	protected = headerRe.ReplaceAllString(protected, "<b>$1</b>")
	// Horizontal rules
	protected = hrRe.ReplaceAllString(protected, "— — —")

	// Blockquotes: group contiguous lines starting with &gt; into <blockquote>
	var bqLines []string
	var resultLines []string
	inBq := false
	for _, line := range strings.Split(protected, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "&gt;") {
			content := strings.TrimPrefix(trimmed, "&gt;")
			content = strings.TrimPrefix(content, " ")
			bqLines = append(bqLines, content)
			inBq = true
		} else {
			if inBq {
				resultLines = append(resultLines, "<blockquote>"+strings.Join(bqLines, "\n")+"</blockquote>")
				bqLines = nil
				inBq = false
			}
			resultLines = append(resultLines, line)
		}
	}
	if inBq {
		resultLines = append(resultLines, "<blockquote>"+strings.Join(bqLines, "\n")+"</blockquote>")
	}
	protected = strings.Join(resultLines, "\n")

	// -------------------------------------------------------------------------
	// Phase 4: Inline markdown structures
	// -------------------------------------------------------------------------
	// Links and images: [text](url), ![alt](url).
	protected = replaceMarkdownDestinations(protected)

	// Bold & Italic
	protected = boldItalicStarRe.ReplaceAllString(protected, "<b><i>$1</i></b>")
	protected = boldItalicUsRe.ReplaceAllString(protected, "<b><i>$1</i></b>")
	protected = boldStarRe.ReplaceAllString(protected, "<b>$1</b>")
	protected = boldUsRe.ReplaceAllString(protected, "<b>$1</b>")
	protected = italicStarRe.ReplaceAllString(protected, "$1<i>$2</i>$3")
	protected = italicUsRe.ReplaceAllString(protected, "$1<i>$2</i>$3")
	protected = strikeRe.ReplaceAllString(protected, "<s>$1</s>")

	// -------------------------------------------------------------------------
	// Phase 5: Restore code tokens
	// -------------------------------------------------------------------------
	if len(codes) > 0 || len(fences) > 0 {
		repls := make([]string, 0, (len(codes)+len(fences))*2)
		for i, c := range codes {
			repls = append(repls, fmt.Sprintf("%sCODE%d\x00", tokenPrefix, i), c)
		}
		for i, f := range fences {
			repls = append(repls, fmt.Sprintf("%sFENCE%d\x00", tokenPrefix, i), f)
		}
		protected = strings.NewReplacer(repls...).Replace(protected)
	}

	// -------------------------------------------------------------------------
	// Phase 6: Whitespace cleanup
	// -------------------------------------------------------------------------
	protected = consecutiveNewlinesRe.ReplaceAllString(protected, "\n\n")
	return strings.TrimSpace(protected)
}

// FormatTextWithMarkdown renders a GitHub markdown body as Telegram HTML.
func replaceMarkdownDestinations(s string) string {
	var out strings.Builder
	for pos := 0; pos < len(s); {
		start := strings.IndexByte(s[pos:], '[')
		if start < 0 {
			out.WriteString(s[pos:])
			break
		}
		start += pos
		image := start > 0 && s[start-1] == '!'
		if image {
			start--
		}

		textEnd := strings.IndexByte(s[start+1:], ']')
		if textEnd < 0 || strings.ContainsAny(s[start+1:start+1+textEnd], "\n") {
			out.WriteString(s[pos : start+1])
			pos = start + 1
			continue
		}
		textEnd += start + 1

		open := textEnd + 1
		if open >= len(s) || s[open] != '(' {
			out.WriteString(s[pos:open])
			pos = open
			continue
		}
		depth := 1
		escaped := false
		closeAt := -1
		for i := open + 1; i < len(s); i++ {
			ch := s[i]
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					closeAt = i
				}
			}
			if closeAt >= 0 {
				break
			}
		}

		textStart := start + 1
		if image {
			textStart++
		}
		text := s[textStart:textEnd]
		if closeAt < 0 || (text == "" && !image) {
			out.WriteString(s[pos:open])
			pos = open
			continue
		}
		destination := s[open+1 : closeAt]
		if strings.ContainsAny(destination, " \t\r\n") || !supportedMarkdownURL(html.UnescapeString(destination)) {
			out.WriteString(s[pos:open])
			pos = open
			continue
		}

		out.WriteString(s[pos:start])
		if image {
			alt := strings.TrimSpace(text)
			if alt == "" {
				alt = "Image"
			}
			fmt.Fprintf(&out, `<a href="%s">🖼️ %s</a>`, destination, alt)
		} else {
			fmt.Fprintf(&out, `<a href="%s">%s</a>`, destination, text)
		}
		pos = closeAt + 1
	}
	return out.String()
}

func supportedMarkdownURL(raw string) bool {
	if strings.HasPrefix(strings.ToLower(raw), "mailto:") {
		return supportedMailtoURL(raw)
	}
	return supportedButtonURL(raw)
}

func supportedButtonURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "tg":
		return true
	default:
		return false
	}
}

func supportedMailtoURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Scheme, "mailto") && (parsed.Opaque != "" || parsed.Path != "")
}

func FormatTextWithMarkdown(body string) string {
	if body == "" {
		return ""
	}
	return MarkdownToTelegramHTML(body)
}

// FormatReleaseBody renders a release notes body. Uses expandable blockquote
// for long release notes so Telegram clients collapse them cleanly without
// breaking unclosed HTML tags.
func FormatReleaseBody(body string) string {
	htmlBody := FormatTextWithMarkdown(body)
	if htmlBody == "" {
		return ""
	}
	lines := strings.Split(htmlBody, "\n")
	const maxLines = 10
	const maxChars = 800
	isLong := len(lines) > maxLines || len(htmlBody) > maxChars

	if !isLong {
		return "<blockquote>" + htmlBody + "</blockquote>"
	}

	return "<blockquote expandable>" + htmlBody + "</blockquote>"
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
	if buttonText == "" || !supportedButtonURL(buttonURL) {
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
