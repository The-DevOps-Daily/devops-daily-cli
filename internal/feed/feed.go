// Package feed keeps a cached copy of the site's RSS feed. Sessions read the
// cache and never wait on the network; a background loop refreshes it.
package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

// UserAgent is sent with every feed request.
const UserAgent = "devops-daily-ssh-site/1.0 (+https://devops-daily.com)"

// Post is one feed item, already cleaned for display.
type Post struct {
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Summary string    `json:"summary"`
	Date    time.Time `json:"date"`
}

// Options configures a Cache. Zero values get sensible defaults.
type Options struct {
	URL      string
	Interval time.Duration // refresh period, default 30 minutes
	Limit    int           // posts kept, default 15
	MaxBytes int64         // largest feed accepted, default 10 MB
	File     string        // optional on-disk copy, loaded at start
	Client   *http.Client
	Logger   *slog.Logger
	Now      func() time.Time
}

// Cache holds the latest posts.
type Cache struct {
	opt     Options
	mu      sync.RWMutex
	posts   []Post
	updated time.Time
	etag    string
	lastMod string
}

// New returns an empty cache.
func New(opt Options) *Cache {
	if opt.Interval <= 0 {
		opt.Interval = 30 * time.Minute
	}
	if opt.Limit <= 0 {
		opt.Limit = 15
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = 10 << 20
	}
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Cache{opt: opt}
}

// Posts returns the cached posts (newest first) and when they were fetched.
// It never blocks on the network. Callers must not modify the slice.
func (c *Cache) Posts() ([]Post, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.posts, c.updated
}

type fileFormat struct {
	Updated      time.Time `json:"updated"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	Posts        []Post    `json:"posts"`
}

// Load reads the on-disk copy, if there is one.
func (c *Cache) Load() error {
	if c.opt.File == "" {
		return nil
	}
	b, err := os.ReadFile(c.opt.File)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var f fileFormat
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("read feed cache: %w", err)
	}
	if len(f.Posts) > c.opt.Limit {
		f.Posts = f.Posts[:c.opt.Limit]
	}
	c.mu.Lock()
	c.posts, c.updated, c.etag, c.lastMod = f.Posts, f.Updated, f.ETag, f.LastModified
	c.mu.Unlock()
	return nil
}

func (c *Cache) save() {
	if c.opt.File == "" {
		return
	}
	c.mu.RLock()
	f := fileFormat{Updated: c.updated, ETag: c.etag, LastModified: c.lastMod, Posts: c.posts}
	c.mu.RUnlock()
	b, err := json.Marshal(f)
	if err == nil {
		tmp := c.opt.File + ".tmp"
		if err = os.WriteFile(tmp, b, 0o600); err == nil {
			err = os.Rename(tmp, filepath.Clean(c.opt.File))
		}
	}
	if err != nil {
		c.opt.Logger.Warn("feed cache not saved", "err", err)
	}
}

// Refresh fetches the feed once. On any failure the cached posts stay.
func (c *Cache) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opt.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.1")
	c.mu.RLock()
	etag, lastMod, have := c.etag, c.lastMod, len(c.posts) > 0
	c.mu.RUnlock()
	if have {
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		if lastMod != "" {
			req.Header.Set("If-Modified-Since", lastMod)
		}
	}

	resp, err := c.opt.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && have {
		c.mu.Lock()
		c.updated = c.opt.Now()
		c.mu.Unlock()
		c.save()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.opt.MaxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > c.opt.MaxBytes {
		return fmt.Errorf("feed is larger than %d bytes", c.opt.MaxBytes)
	}
	posts, err := Parse(bytes.NewReader(data), c.opt.Limit)
	if err != nil {
		return err
	}
	if len(posts) == 0 {
		return errors.New("feed has no usable items")
	}
	c.mu.Lock()
	c.posts = posts
	c.updated = c.opt.Now()
	c.etag = resp.Header.Get("ETag")
	c.lastMod = resp.Header.Get("Last-Modified")
	c.mu.Unlock()
	c.save()
	return nil
}

// Run refreshes the cache until ctx ends: right away when the cache is empty
// or stale, then every Interval. Failures retry sooner, backing off from one
// minute up to Interval.
func (c *Cache) Run(ctx context.Context) {
	var delay time.Duration
	if posts, updated := c.Posts(); len(posts) > 0 {
		if age := c.opt.Now().Sub(updated); age >= 0 && age < c.opt.Interval {
			delay = c.opt.Interval - age
		}
	}
	failures := 0
	for {
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		err := c.Refresh(rctx)
		cancel()
		if err != nil {
			failures++
			delay = retryDelay(failures, c.opt.Interval)
			c.opt.Logger.Warn("feed refresh failed", "err", err, "retry_in", delay)
			continue
		}
		failures = 0
		delay = c.opt.Interval
		posts, _ := c.Posts()
		c.opt.Logger.Info("feed refreshed", "posts", len(posts))
	}
}

func retryDelay(failures int, max time.Duration) time.Duration {
	d := time.Minute
	for i := 1; i < failures && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

type rssDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// Parse reads an RSS 2.0 document and returns up to limit posts, newest
// first. Items without a title or an http(s) link are skipped.
func Parse(r io.Reader, limit int) ([]Post, error) {
	var doc rssDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	posts := make([]Post, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		title := text.Line(html.UnescapeString(it.Title), 200)
		link := strings.TrimSpace(it.Link)
		u, err := url.Parse(link)
		if title == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			continue
		}
		posts = append(posts, Post{
			Title:   title,
			URL:     text.Line(link, 300),
			Summary: Summarize(it.Description, 320),
			Date:    parseDate(it.PubDate),
		})
	}
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].Date.After(posts[j].Date) })
	if limit > 0 && len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// Summarize turns an item description (plain text or HTML) into one clean
// line of at most max runes.
func Summarize(s string, max int) string {
	s = tagRE.ReplaceAllString(s, " ")
	return text.Line(html.UnescapeString(s), max)
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 MST", "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
