// Package text cleans strings before they reach a visitor's terminal.
package text

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Clean removes characters that a terminal could interpret: C0 and C1
// control codes (so no escape sequences survive), DEL, invalid UTF-8 and
// bidirectional overrides. Tabs become spaces. Em and en dashes become
// hyphens. Newlines are kept only when keepNewlines is true; otherwise they
// become spaces.
func Clean(s string, keepNewlines bool) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == utf8.RuneError && size <= 1:
			// invalid byte: drop it
		case r == '\n' || r == '\r':
			if keepNewlines && r == '\n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// control code: drop it
		case (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0x200e || r == 0x200f:
			// bidi control: drop it
		case r == '\u2014' || r == '\u2013':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Line cleans s, folds all whitespace into single spaces and shortens the
// result to at most max runes.
func Line(s string, max int) string {
	s = strings.Join(strings.Fields(Clean(s, false)), " ")
	return Truncate(s, max)
}

// Truncate shortens s to at most max runes, cutting at a word boundary when
// one is close and adding "...".
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	if max <= 3 {
		return string([]rune(s)[:max])
	}
	r := []rune(s)[:max-3]
	cut := len(r)
	for j := len(r) - 1; j > len(r)*2/3; j-- {
		if unicode.IsSpace(r[j]) {
			cut = j
			break
		}
	}
	return strings.TrimRightFunc(string(r[:cut]), unicode.IsSpace) + "..."
}
