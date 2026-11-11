package text

import "testing"

func TestCleanDropsControlCodes(t *testing.T) {
	in := "safe\x1b[2J\x1b]0;title\x07 text\x9b31m\u202edone\u2014ok"
	got := Clean(in, false)
	want := "safe[2J]0;title text31mdone-ok"
	if got != want {
		t.Fatalf("Clean = %q, want %q", got, want)
	}
}

func TestCleanNewlines(t *testing.T) {
	if got := Clean("a\r\nb\tc", true); got != "a\nb    c" {
		t.Fatalf("keep newlines: %q", got)
	}
	if got := Clean("a\nb", false); got != "a b" {
		t.Fatalf("fold newlines: %q", got)
	}
	if got := Clean("bad\xffbyte", false); got != "badbyte" {
		t.Fatalf("invalid utf-8: %q", got)
	}
}

func TestLineAndTruncate(t *testing.T) {
	if got := Line("  many   spaces\n here ", 100); got != "many spaces here" {
		t.Fatalf("Line = %q", got)
	}
	got := Truncate("the quick brown fox jumps over the lazy dog", 20)
	if got != "the quick brown..." {
		t.Fatalf("Truncate = %q", got)
	}
	if got := Truncate("short", 20); got != "short" {
		t.Fatalf("Truncate short = %q", got)
	}
}
