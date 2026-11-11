package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func rss(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>DevOps Daily</title><link>https://devops-daily.com</link>
<atom:link href="https://devops-daily.com/feed.xml" rel="self" type="application/rss+xml"/>
` + strings.Join(items, "\n") + `
</channel></rss>`
}

func item(title, link, desc, date string) string {
	return fmt.Sprintf(`<item><title><![CDATA[%s]]></title><link>%s</link><description><![CDATA[%s]]></description><pubDate>%s</pubDate><content:encoded><![CDATA[<p>long body</p>]]></content:encoded></item>`, title, link, desc, date)
}

func TestParseSortsCleansAndLimits(t *testing.T) {
	doc := rss(
		item("Older", "https://devops-daily.com/posts/older", "old", "Mon, 02 Nov 2026 09:00:00 GMT"),
		item("Newest \u009b31mred\u202e", "https://devops-daily.com/posts/new", "<p>Has <b>HTML</b> &amp; entities</p>", "Tue, 10 Nov 2026 09:00:00 GMT"),
		item("Middle", "https://devops-daily.com/posts/mid", "mid", "Thu, 05 Nov 2026 21:00:00 GMT"),
		item("Bad link", "javascript:alert(1)", "x", "Thu, 05 Nov 2026 21:00:00 GMT"),
		item("", "https://devops-daily.com/posts/untitled", "x", "Thu, 05 Nov 2026 21:00:00 GMT"),
	)
	posts, err := Parse(strings.NewReader(doc), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
	if posts[0].Title != "Newest red" {
		t.Fatalf("escape codes not removed: %q", posts[0].Title)
	}
	if posts[0].Summary != "Has HTML & entities" {
		t.Fatalf("summary = %q", posts[0].Summary)
	}
	if posts[1].Title != "Middle" {
		t.Fatalf("not sorted newest first: %q", posts[1].Title)
	}
	if want := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC); !posts[0].Date.Equal(want) {
		t.Fatalf("date = %v", posts[0].Date)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse(strings.NewReader("<rss><channel><item>"), 15); err == nil {
		t.Fatal("expected an error for a truncated document")
	}
}

func TestRefreshKeepsOldPostsOnFailure(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, rss(item("One", "https://devops-daily.com/posts/one", "d", "Tue, 10 Nov 2026 09:00:00 GMT")))
	}))
	defer srv.Close()

	c := New(Options{URL: srv.URL})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("expected an error from a 502")
	}
	posts, updated := c.Posts()
	if len(posts) != 1 || posts[0].Title != "One" || updated.IsZero() {
		t.Fatalf("cache lost its posts: %+v", posts)
	}
}

func TestPostsNeverWaitsForASlowFeed(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		fmt.Fprint(w, rss(item("Late", "https://devops-daily.com/posts/late", "d", "Tue, 10 Nov 2026 09:00:00 GMT")))
	}))
	defer srv.Close()
	defer close(release)

	c := New(Options{URL: srv.URL})
	go c.Refresh(context.Background())
	time.Sleep(50 * time.Millisecond) // let the request start

	done := make(chan struct{})
	go func() {
		c.Posts()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Posts blocked while a refresh was in flight")
	}
}

func TestRefreshHonoursSizeLimitAndNotModified(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, rss(item("One", "https://devops-daily.com/posts/one", "d", "Tue, 10 Nov 2026 09:00:00 GMT")))
	}))
	defer srv.Close()

	small := New(Options{URL: srv.URL, MaxBytes: 64})
	if err := small.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("expected a size error, got %v", err)
	}

	now := time.Unix(1_800_000_000, 0)
	c := New(Options{URL: srv.URL, Now: func() time.Time { return now }})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("304 handling: %v", err)
	}
	posts, updated := c.Posts()
	if len(posts) != 1 || !updated.Equal(now) {
		t.Fatalf("after 304: %d posts, updated %v", len(posts), updated)
	}
}

func TestCacheFileRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, rss(item("Saved", "https://devops-daily.com/posts/saved", "d", "Tue, 10 Nov 2026 09:00:00 GMT")))
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "feed.json")
	a := New(Options{URL: srv.URL, File: file})
	if err := a.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := New(Options{URL: "http://127.0.0.1:1/unused", File: file})
	if err := b.Load(); err != nil {
		t.Fatal(err)
	}
	posts, _ := b.Posts()
	if len(posts) != 1 || posts[0].Title != "Saved" {
		t.Fatalf("loaded %+v", posts)
	}
}

func TestRunRefreshesOnSchedule(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, rss(item("One", "https://devops-daily.com/posts/one", "d", "Tue, 10 Nov 2026 09:00:00 GMT")))
	}))
	defer srv.Close()

	c := New(Options{URL: srv.URL, Interval: 40 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()
	if n := hits.Load(); n < 3 {
		t.Fatalf("expected at least 3 refreshes in 150ms with a 40ms interval, got %d", n)
	}
}

func TestRetryDelay(t *testing.T) {
	cases := map[int]time.Duration{1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 10: 30 * time.Minute}
	for n, want := range cases {
		if got := retryDelay(n, 30*time.Minute); got != want {
			t.Errorf("retryDelay(%d) = %v, want %v", n, got, want)
		}
	}
}
