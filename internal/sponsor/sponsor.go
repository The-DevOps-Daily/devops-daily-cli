// Package sponsor reads the one-line sponsor slot from a small config file.
// The file is re-read when it changes, so the slot can be edited on the
// server without a rebuild or a restart.
//
// File format, one key=value per line, # starts a comment:
//
//	name=Example
//	text=What the sponsor does, in a few words
//	url=https://example.com
//
// An empty or missing file hides the line.
package sponsor

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

// Sponsor is what the slot shows.
type Sponsor struct {
	Name string
	Text string
	URL  string
}

// Empty reports whether there is nothing to show.
func (s Sponsor) Empty() bool { return s.Name == "" }

// Parse reads the key=value format. Unknown keys are ignored.
func Parse(r io.Reader) (Sponsor, error) {
	var s Sponsor
	sc := bufio.NewScanner(io.LimitReader(r, 4096))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "name":
			s.Name = text.Line(v, 60)
		case "text", "tagline":
			s.Text = text.Line(v, 120)
		case "url":
			s.URL = text.Line(v, 200)
		}
	}
	if s.URL != "" && !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://") {
		s.URL = ""
	}
	return s, sc.Err()
}

// Source caches the parsed file and checks it for changes at most once per
// interval.
type Source struct {
	path  string
	every time.Duration
	now   func() time.Time

	mu      sync.Mutex
	checked time.Time
	modTime time.Time
	size    int64
	cur     Sponsor
}

// NewSource returns a Source for path that re-checks the file at most once
// every interval.
func NewSource(path string, every time.Duration) *Source {
	return &Source{path: path, every: every, now: time.Now, size: -1}
}

// Get returns the current sponsor. A missing or unreadable file gives an
// empty Sponsor.
func (s *Source) Get() Sponsor {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.checked.IsZero() && now.Sub(s.checked) < s.every {
		return s.cur
	}
	s.checked = now
	fi, err := os.Stat(s.path)
	if err != nil || !fi.Mode().IsRegular() {
		s.cur, s.modTime, s.size = Sponsor{}, time.Time{}, -1
		return s.cur
	}
	if fi.ModTime().Equal(s.modTime) && fi.Size() == s.size {
		return s.cur
	}
	f, err := os.Open(s.path)
	if err != nil {
		s.cur = Sponsor{}
		return s.cur
	}
	defer f.Close()
	sp, err := Parse(f)
	if err != nil {
		return s.cur
	}
	s.cur, s.modTime, s.size = sp, fi.ModTime(), fi.Size()
	return s.cur
}
