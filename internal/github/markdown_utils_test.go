package github

import (
	"strings"
	"testing"
)

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"a < b & c > d", "a &lt; b &amp; c &gt; d"},
		{"quotes \"here\"", "quotes &#34;here&#34;"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := EscapeHTML(tt.input); got != tt.expected {
				t.Errorf("EscapeHTML() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormatRepo(t *testing.T) {
	got := FormatRepo("owner/my_repo")
	want := `<a href="https://github.com/owner/my_repo">owner/my_repo</a>`
	if got != want {
		t.Errorf("FormatRepo() = %v, want %v", got, want)
	}
}

func TestFormatMessageRejectsUnsupportedButtonURL(t *testing.T) {
	message, markup := FormatMessageWithButton("message", "Open", "javascript:alert(1)")
	if message != "message" || markup != nil {
		t.Fatalf("unsupported button URL produced markup: %+v", markup)
	}
}

func TestFormatUser(t *testing.T) {
	got := FormatUser("user_name")
	want := `<a href="https://github.com/user_name">user_name</a>`
	if got != want {
		t.Errorf("FormatUser() = %v, want %v", got, want)
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		name string
		sha  string
		want string
	}{
		{name: "empty", sha: "", want: ""},
		{name: "short", sha: "abc", want: "abc"},
		{name: "exactly seven", sha: "1234567", want: "1234567"},
		{name: "long", sha: "1234567890abcdef", want: "1234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortSHA(tt.sha); got != tt.want {
				t.Fatalf("ShortSHA() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatReleaseBodyLongUsesExpandableBlockquote(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, "line")
	}
	got := FormatReleaseBody(strings.Join(lines, "\n"))

	if strings.Contains(got, "||") {
		t.Errorf("FormatReleaseBody() should not emit markdown spoilers in HTML mode")
	}
	if !strings.HasPrefix(got, "<blockquote expandable>") || !strings.HasSuffix(got, "</blockquote>") {
		t.Errorf("FormatReleaseBody() = %q, want expandable blockquote wrapping", got)
	}
}

func TestFormatTextWithMarkdownRendersLinksAndEscapesText(t *testing.T) {
	in := "See [release.v2.1](https://github.com/o/r/releases/tag/v2.1) for <details>."
	got := FormatTextWithMarkdown(in)

	if !strings.Contains(got, `<a href="https://github.com/o/r/releases/tag/v2.1">release.v2.1</a>`) {
		t.Fatalf("FormatTextWithMarkdown() = %q, want link preserved", got)
	}
	if strings.Contains(got, "<details>") {
		t.Fatalf("FormatTextWithMarkdown() = %q, want raw HTML escaped", got)
	}
	if !strings.Contains(got, "&lt;details&gt;") {
		t.Fatalf("FormatTextWithMarkdown() = %q, want escaped angle brackets", got)
	}
}

func TestFormatTextWithMarkdownKeepsCodeVerbatim(t *testing.T) {
	got := FormatTextWithMarkdown("Run `go test <here>` now")
	if !strings.Contains(got, "<code>go test &lt;here&gt;</code>") {
		t.Fatalf("code spans should render as escaped <code>, got %q", got)
	}
}

func TestMarkdownToTelegramHTMLHeaders(t *testing.T) {
	in := "# Title 1\n## Subtitle 2\n### Section 3"
	got := MarkdownToTelegramHTML(in)
	for _, want := range []string{"<b>Title 1</b>", "<b>Subtitle 2</b>", "<b>Section 3</b>"} {
		if !strings.Contains(got, want) {
			t.Errorf("MarkdownToTelegramHTML(%q) missing %q, got: %q", in, want, got)
		}
	}
}

func TestMarkdownToTelegramHTMLBulletsAndNoItalicCorruption(t *testing.T) {
	in := "* feat: update_user_session function\n* fix: another_snake_case_ident"
	got := MarkdownToTelegramHTML(in)

	// Bullets must be converted to •
	if !strings.Contains(got, "• feat: update_user_session function") {
		t.Errorf("Expected clean bullet point, got: %q", got)
	}
	// Must NOT corrupt snake_case with <i> tags
	if strings.Contains(got, "<i>") {
		t.Errorf("Must not corrupt snake_case with italics, got: %q", got)
	}
}

func TestMarkdownToTelegramHTMLTaskLists(t *testing.T) {
	in := "- [x] Finished task\n- [ ] Pending task"
	got := MarkdownToTelegramHTML(in)
	if !strings.Contains(got, "• ✅ Finished task") {
		t.Errorf("Expected completed checklist icon, got: %q", got)
	}
	if !strings.Contains(got, "• ◻️ Pending task") {
		t.Errorf("Expected pending checklist icon, got: %q", got)
	}
}

func TestMarkdownToTelegramHTMLBlockquotes(t *testing.T) {
	in := "> First quoted line\n> Second quoted line\n\nNormal line"
	got := MarkdownToTelegramHTML(in)
	want := "<blockquote>First quoted line\nSecond quoted line</blockquote>\n\nNormal line"
	if got != want {
		t.Errorf("MarkdownToTelegramHTML() = %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLFencedCodeLanguage(t *testing.T) {
	in := "```python\ndef hello():\n    print(\"world\")\n```"
	got := MarkdownToTelegramHTML(in)
	want := "<pre><code class=\"language-python\">def hello():\n    print(&#34;world&#34;)</code></pre>"
	if got != want {
		t.Errorf("MarkdownToTelegramHTML() = %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLFencedCodeWithFourBackticks(t *testing.T) {
	got := MarkdownToTelegramHTML("````go\ncode\n````")
	want := "<pre><code class=\"language-go\">code</code></pre>"
	if got != want {
		t.Fatalf("four-backtick fence = %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLRejectsPlaceholderCollision(t *testing.T) {
	in := "before\x00CODE0\x00 after `secret`"
	want := "before\x00CODE0\x00 after <code>secret</code>"
	if got := MarkdownToTelegramHTML(in); got != want {
		t.Fatalf("placeholder collision changed text: got %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLRejectsUnsupportedURLSchemes(t *testing.T) {
	for _, in := range []string{
		"[click](javascript:alert(1))",
		"![image](data:text/html,payload)",
		"[missing host](https://)",
		"[missing target](mailto:)",
	} {
		got := MarkdownToTelegramHTML(in)
		if strings.Contains(got, "<a href=") {
			t.Errorf("unsupported URL became an active link: %q", got)
		}
	}
}

func TestMarkdownToTelegramHTMLSupportsBalancedParenthesesInURL(t *testing.T) {
	got := MarkdownToTelegramHTML("[wiki](https://example.com/a(b))")
	want := `<a href="https://example.com/a(b)">wiki</a>`
	if got != want {
		t.Fatalf("balanced URL = %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLPreservesEscapedQueryURL(t *testing.T) {
	got := MarkdownToTelegramHTML("[query](https://example.com/?a=1&b=2)")
	want := `<a href="https://example.com/?a=1&amp;b=2">query</a>`
	if got != want {
		t.Fatalf("query URL = %q, want %q", got, want)
	}
}

func TestMarkdownToTelegramHTMLImages(t *testing.T) {
	in := "Look at ![dashboard screenshot](https://example.com/dash.png)!"
	got := MarkdownToTelegramHTML(in)
	want := `Look at <a href="https://example.com/dash.png">🖼️ dashboard screenshot</a>!`
	if got != want {
		t.Errorf("MarkdownToTelegramHTML() = %q, want %q", got, want)
	}
}
