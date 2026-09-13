package ui

import (
	"testing"

	gh "github.com/google/go-github/v91/github"
)

func TestCompactButtonText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short name unchanged",
			input: "owner/repo",
			want:  "owner/repo",
		},
		{
			name:  "exactly 42 runes unchanged",
			input: "123456789012345678901234567890123456789012",
			want:  "123456789012345678901234567890123456789012",
		},
		{
			name:  "43 runes truncated to 41 + ellipsis",
			input: "1234567890123456789012345678901234567890123",
			want:  "12345678901234567890123456789012345678901…",
		},
		{
			name:  "unicode characters correctly counted by runes",
			input: "🚀📦🎉✨🔥🌟⚡️🎯🎨🏆🚀📦🎉✨🔥🌟⚡️🎯🎨🏆🚀📦🎉✨🔥🌟⚡️🎯🎨🏆ABC",
			// 30 emoji (each 1 rune) + "ABC" = 33 runes <= 42
			want: "🚀📦🎉✨🔥🌟⚡️🎯🎨🏆🚀📦🎉✨🔥🌟⚡️🎯🎨🏆🚀📦🎉✨🔥🌟⚡️🎯🎨🏆ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompactButtonText(tt.input)
			if got != tt.want {
				t.Fatalf("CompactButtonText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithStyle(t *testing.T) {
	btn := Callback("test", "data", WithStyle(StyleDanger))
	if btn.Style != StyleDanger {
		t.Fatalf("btn.Style = %q, want %q", btn.Style, StyleDanger)
	}

	btnDefault := Callback("test", "data", WithStyle("invalid_style"))
	if btnDefault.Style != "" {
		t.Fatalf("btn.Style = %q, want empty for invalid", btnDefault.Style)
	}
}

func TestRowAndMarkup(t *testing.T) {
	b1 := Callback("one", "data1")
	b2 := Callback("two", "data2")
	row := Row(b1, b2)
	if len(row) != 2 {
		t.Fatalf("Row length = %d, want 2", len(row))
	}

	markup := Markup(row)
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 2 {
		t.Fatalf("Markup structure mismatch: %+v", markup)
	}
}

func TestBackButton(t *testing.T) {
	btn := BackButton("back:target")
	if btn.Text != "Back" || btn.CallbackData != "back:target" {
		t.Fatalf("BackButton mismatch: %+v", btn)
	}
	if btn.Style != StylePrimary {
		t.Fatalf("BackButton style = %q, want %q", btn.Style, StylePrimary)
	}
}

func TestRepoPageNav(t *testing.T) {
	pageBuilder := func(p int) string { return "page:" + string(rune('0'+p)) }

	// Single page: no navigation
	singlePage := &gh.Response{
		Response: nil,
	}
	if nav := RepoPageNav(1, singlePage, pageBuilder); nav != nil {
		t.Fatalf("RepoPageNav for single page = %+v, want nil", nav)
	}

	// Multi-page: page 2 of 5
	multiPage := &gh.Response{
		PrevPage:  1,
		NextPage:  3,
		FirstPage: 1,
		LastPage:  5,
	}
	nav := RepoPageNav(2, multiPage, pageBuilder)
	if len(nav) == 0 {
		t.Fatal("RepoPageNav returned empty row for multi-page")
	}

	// Must have '<' previous button and '>' next button
	hasPrev := false
	hasNext := false
	for _, b := range nav {
		if b.Text == "<" {
			hasPrev = true
		}
		if b.Text == ">" {
			hasNext = true
		}
	}
	if !hasPrev || !hasNext {
		t.Fatalf("RepoPageNav missing nav buttons: hasPrev=%v, hasNext=%v in %+v", hasPrev, hasNext, nav)
	}
}
