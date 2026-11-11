package text

import "testing"

func TestCleanDropsControlCodes(t *testing.T) {
	in := "safe\x1b[2J\x1b]0;title\x07 text\x9b31m\u202edone\u2014ok"
	got := Clean(in, false)
	want := "safe textdone-ok"
	if got != want {
		t.Fatalf("Clean = %q, want %q", got, want)
	}
}

func TestCleanStripsWholeEscapeSequences(t *testing.T) {
	cases := map[string]string{
		"a\x1b[1;31mred\x1b[0m b":                                "ared b",
		"x\x1b]8;;https://evil.example\x1b\\link\x1b]8;;\x1b\\y": "xlinky",
		"title\x1b]2;owned\x07!":                                 "title!",
		"q\x1bPdcs data\x1b\\r":                                  "qr",
		"\x1b":                                                   "",
		"\x1b[":                                                  "",
		"keep\x1b[12\nnext":                                      "keep next",
		"c1\u009b2Jcsi":                                          "c1csi",
	}
	for in, want := range cases {
		if got := Clean(in, false); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
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
