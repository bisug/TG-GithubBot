package github

import (
	"strings"
	"testing"
)

// wellFormedEnding checks that a truncated message ends cleanly: it must not
// end inside a tag, and every tag opened before the cut must be closed by the
// appended closers.
func wellFormedEnding(t *testing.T, out string) {
	t.Helper()
	// No dangling '<' without a closing '>': the last tag must be complete.
	if i := strings.LastIndex(out, "<"); i >= 0 && !strings.Contains(out[i:], ">") {
		t.Fatalf("output ends inside an incomplete tag: %q", out[maxInt(0, len(out)-40):])
	}
	// No dangling entity.
	if i := strings.LastIndex(out, "&"); i >= 0 && !strings.Contains(out[i:], ";") {
		t.Fatalf("output ends inside an incomplete entity: %q", out[maxInt(0, len(out)-40):])
	}
	// Every opened tag must be closed.
	depth := 0
	for _, m := range htmlTagRe.FindAllStringSubmatch(out, -1) {
		if strings.HasSuffix(strings.TrimSpace(m[3]), "/") {
			continue
		}
		if m[1] == "/" {
			depth--
		} else {
			depth++
		}
		if depth < 0 {
			t.Fatalf("closing tag without opener in %q", out)
		}
	}
	if depth != 0 {
		t.Fatalf("unclosed tags remain (depth %d) in %q", depth, out[maxInt(0, len(out)-60):])
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestTruncateTelegramHTMLShortMessageUnchanged(t *testing.T) {
	msg := "<b>hello</b> world &amp; friends"
	if got := truncateTelegramHTML(msg, 4096); got != msg {
		t.Fatalf("short message must be unchanged, got %q", got)
	}
}

func TestTruncateTelegramHTMLCapsLength(t *testing.T) {
	msg := strings.Repeat("a", 5000)
	got := truncateTelegramHTML(msg, 4096)
	if n := len([]rune(got)); n > 4096 {
		t.Fatalf("expected at most 4096 runes, got %d", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got[len(got)-5:])
	}
}

func TestTruncateTelegramHTMLClosesOpenTags(t *testing.T) {
	msg := "<b>bold <i>italic</i> " + strings.Repeat("x", 5000)
	got := truncateTelegramHTML(msg, 4096)
	if n := len([]rune(got)); n > 4096 {
		t.Fatalf("expected at most 4096 runes, got %d", n)
	}
	if !strings.HasSuffix(got, "</b>") {
		t.Fatalf("expected the open <b> to be closed, got suffix %q", got[len(got)-10:])
	}
	wellFormedEnding(t, got)
}

func TestTruncateTelegramHTMLCutInsideTag(t *testing.T) {
	// Force the 4096-rune boundary to fall in the middle of a <b> tag.
	prefix := "<blockquote>" + strings.Repeat("a", 4080) + "</blockquote>"
	msg := prefix + strings.Repeat("b", 100)
	got := truncateTelegramHTML(msg, 4096)
	wellFormedEnding(t, got)
	if n := len([]rune(got)); n > 4096 {
		t.Fatalf("expected at most 4096 runes, got %d", n)
	}
}

func TestTruncateTelegramHTMLSmallLimit(t *testing.T) {
	for limit := 1; limit <= 8; limit++ {
		msg := "<b><i>abcdefghij</i></b>"
		got := truncateTelegramHTML(msg, limit)
		if n := len([]rune(got)); n > limit {
			t.Errorf("limit %d produced %d runes: %q", limit, n, got)
		}
		wellFormedEnding(t, got)
	}
}

func TestTruncateTelegramHTMLCutInsideEntity(t *testing.T) {
	// Force the boundary to fall inside an escaped entity: the "&amp;" starts
	// at rune 4092, so the initial cut at 4095 lands mid-entity ("&am").
	prefix := strings.Repeat("a", 4092) + "&amp;"
	msg := prefix + strings.Repeat("b", 100)
	got := truncateTelegramHTML(msg, 4096)
	wellFormedEnding(t, got)
	if strings.Contains(got, "&am") && !strings.Contains(strings.Split(got, "&am")[1], ";") {
		t.Fatalf("incomplete entity left in output")
	}
}
