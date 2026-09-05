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

func TestFormatReleaseBodyLongUsesSpoiler(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, "line")
	}
	got := FormatReleaseBody(strings.Join(lines, "\n"))

	if strings.Contains(got, "||") {
		t.Errorf("FormatReleaseBody() should not emit markdown spoilers in HTML mode")
	}
	if !strings.Contains(got, "<blockquote>") || !strings.Contains(got, "<tg-spoiler>") {
		t.Errorf("FormatReleaseBody() = %q, want blockquote + spoiler", got)
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
