package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/feed"
)

func TestOptionsHeaderStartAndArticleCache(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	posts := []feed.Post{{Title: "Post", URL: "https://devops-daily.com/posts/post", Summary: "Summary"}}
	calls := 0
	m := New(lipgloss.NewRenderer(io.Discard), Deps{Now: func() time.Time { return now }, Header: "devops-daily.com", Start: "posts", Posts: func() ([]feed.Post, time.Time) { return posts, now }, Article: func(feed.Post, int) ([]string, error) { calls++; return []string{"Full article"}, nil }})
	m, _ = send(m, key("enter"))
	if !strings.Contains(m.View(), "devops-daily.com") || !strings.Contains(m.View(), "Full article") {
		t.Fatalf("options not rendered: %s", m.View())
	}
	_ = m.View()
	if calls != 1 {
		t.Fatalf("article rendered %d times", calls)
	}
}

func TestOpenURLOption(t *testing.T) {
	now := time.Now()
	posts := []feed.Post{{Title: "Post", URL: "https://devops-daily.com/posts/post"}}
	var opened string
	m := New(lipgloss.NewRenderer(io.Discard), Deps{Now: func() time.Time { return now }, Start: "posts", Posts: func() ([]feed.Post, time.Time) { return posts, now }, OpenURL: func(url string) error { opened = url; return nil }})
	m, _ = send(m, key("enter"), key("o"))
	if opened != posts[0].URL || !strings.Contains(m.View(), "Opened in your browser.") || !strings.Contains(m.View(), "open") {
		t.Fatalf("open option failed: %q %s", opened, m.View())
	}
	m, _ = send(m, key("j"))
	if strings.Contains(m.View(), "Opened in your browser.") {
		t.Fatal("open note was not cleared")
	}
}

func TestOpenDoesNothingWithoutHandler(t *testing.T) {
	now := time.Now()
	posts := []feed.Post{{Title: "Post", URL: "https://devops-daily.com/posts/post"}}
	m := New(lipgloss.NewRenderer(io.Discard), Deps{Now: func() time.Time { return now }, Start: "posts", Posts: func() ([]feed.Post, time.Time) { return posts, now }})
	m, _ = send(m, key("enter"), key("o"))
	if strings.Contains(m.View(), "Opened in your browser.") {
		t.Fatal("o should be inert without OpenURL")
	}
}
