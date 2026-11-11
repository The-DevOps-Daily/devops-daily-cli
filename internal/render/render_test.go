package render

import (
	"strings"
	"testing"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

func TestLinks(t *testing.T) {
	markdown := "[One](https://example.com/a) <https://example.com/b> [Again](https://example.com/a) [relative](/x)\n\n```\n[hidden](https://example.com/c)\n```"
	links := Links(markdown, "https://devops-daily.com")
	if len(links) != 2 || links[0].URL != "https://example.com/a" || links[1].URL != "https://example.com/b" {
		t.Fatalf("unexpected links: %#v", links)
	}
}

func TestArticleNotty(t *testing.T) {
	article, err := Article(catalog.PostMeta{Title: "Title", Date: "2026-01-01", URL: "https://devops-daily.com/posts/x"}, "[Link](https://example.com)", 80, "notty")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Title", "Links", "https://example.com"} {
		if !strings.Contains(article, want) {
			t.Errorf("article missing %q: %s", want, article)
		}
	}
	if strings.Contains(article, "\x1b") {
		t.Fatalf("article contains ANSI: %q", article)
	}
}
