package sponsor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	s, err := Parse(strings.NewReader("# comment\nname = Example\u2014Co\ntext=Fast \x1b[1mthings\nurl=https://example.com/x\nother=ignored\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Sponsor{Name: "Example-Co", Text: "Fast things", URL: "https://example.com/x"}
	if s != want {
		t.Fatalf("got %+v, want %+v", s, want)
	}
	s, _ = Parse(strings.NewReader("name=X\nurl=javascript:alert(1)\n"))
	if s.URL != "" {
		t.Fatalf("non-http URL kept: %q", s.URL)
	}
}

func TestSourceReloadsWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sponsor.conf")
	clock := time.Unix(1_800_000_000, 0)
	src := NewSource(path, 10*time.Second)
	src.now = func() time.Time { return clock }

	if !src.Get().Empty() {
		t.Fatal("missing file should hide the slot")
	}
	if err := os.WriteFile(path, []byte("name=First\nurl=https://first.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(11 * time.Second)
	if got := src.Get().Name; got != "First" {
		t.Fatalf("after write: %q", got)
	}

	if err := os.WriteFile(path, []byte("name=Second sponsor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := src.Get().Name; got != "First" {
		t.Fatalf("re-read before the interval: %q", got)
	}
	clock = clock.Add(11 * time.Second)
	if got := src.Get().Name; got != "Second sponsor" {
		t.Fatalf("after change: %q", got)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(11 * time.Second)
	if !src.Get().Empty() {
		t.Fatal("removed file should hide the slot")
	}
}
