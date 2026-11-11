package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const defaultBaseURL = "https://devops-daily.com/cli/v1/"
const maxFileBytes int64 = 32 << 20

var ErrNewerSchema = errors.New("content uses a newer schema; update the CLI")
var fileNameRE = regexp.MustCompile(`^[a-z0-9-]+(/[a-z0-9-]+)?\.json$`)

type cacheMeta struct {
	ETag      string    `json:"etag,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
	SyncedAt  time.Time `json:"syncedAt"`
}

type SyncResult struct {
	FilesDownloaded int
	BytesDownloaded int64
	Changed         bool
}

type Store struct {
	Dir       string
	BaseURL   string
	UserAgent string
	Client    *http.Client
	Now       func() time.Time
	MaxAge    time.Duration

	mu        sync.Mutex
	posts     *PostsIndex
	quiz      *Quizzes
	decks     *Decks
	checks    *Checklists
	incidents *Incidents
	games     *Games
	sponsors  *Sponsors
	manifest  *Manifest
	bodies    map[string]PostBodies
}

func New(dir, baseURL, userAgent string) *Store {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Store{Dir: dir, BaseURL: baseURL, UserAgent: userAgent, Client: &http.Client{Timeout: 30 * time.Second}, Now: time.Now, MaxAge: 6 * time.Hour}
}

func ValidBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	switch strings.Trim(u.Hostname(), "[]") {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *Store) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

func (s *Store) readMeta() (cacheMeta, error) {
	var meta cacheMeta
	b, err := os.ReadFile(filepath.Join(s.Dir, "meta.json"))
	if errors.Is(err, os.ErrNotExist) {
		return meta, nil
	}
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(b, &meta)
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".devops-daily-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err == nil {
		err = f.Chmod(0o600)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSON(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}
func decodeManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Schema != Schema {
		return nil, ErrNewerSchema
	}
	return &m, nil
}

func (s *Store) request(ctx context.Context, name, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+name, nil)
	if err != nil {
		return nil, err
	}
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return s.client().Do(req)
}

func (s *Store) Sync(ctx context.Context) (SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidBaseURL(s.BaseURL) {
		return SyncResult{}, fmt.Errorf("invalid data URL: %s", s.BaseURL)
	}
	meta, err := s.readMeta()
	if err != nil {
		return SyncResult{}, fmt.Errorf("read cache metadata: %w", err)
	}
	resp, err := s.request(ctx, "index.json", meta.ETag)
	if err != nil {
		return SyncResult{}, err
	}
	defer resp.Body.Close()
	now := s.now()
	if resp.StatusCode == http.StatusNotModified {
		meta.CheckedAt = now
		return SyncResult{}, writeJSON(filepath.Join(s.Dir, "meta.json"), meta)
	}
	if resp.StatusCode != http.StatusOK {
		return SyncResult{}, fmt.Errorf("download index.json: HTTP %d", resp.StatusCode)
	}
	index, err := readLimited(resp.Body, maxFileBytes)
	if err != nil {
		return SyncResult{}, fmt.Errorf("download index.json: %w", err)
	}
	manifest, err := decodeManifest(index)
	if err != nil {
		return SyncResult{}, err
	}
	for name := range manifest.Files {
		if !fileNameRE.MatchString(name) {
			return SyncResult{}, fmt.Errorf("invalid manifest file name %q", name)
		}
	}
	pending := make(map[string][]byte)
	var result SyncResult
	for _, name := range sortedFiles(manifest.Files) {
		info := manifest.Files[name]
		if info.Bytes < 0 || info.Bytes > maxFileBytes {
			return SyncResult{}, fmt.Errorf("file %s is too large", name)
		}
		path := filepath.Join(s.Dir, filepath.FromSlash(name))
		if b, err := os.ReadFile(path); err == nil && hash(b) == strings.ToLower(info.SHA256) {
			continue
		}
		fileResp, err := s.request(ctx, name, "")
		if err != nil {
			return SyncResult{}, err
		}
		b, readErr := readLimited(fileResp.Body, info.Bytes)
		fileResp.Body.Close()
		if readErr != nil {
			return SyncResult{}, fmt.Errorf("download %s: %w", name, readErr)
		}
		if fileResp.StatusCode != http.StatusOK {
			return SyncResult{}, fmt.Errorf("download %s: HTTP %d", name, fileResp.StatusCode)
		}
		if hash(b) != strings.ToLower(info.SHA256) {
			return SyncResult{}, fmt.Errorf("file %s failed its SHA-256 check", name)
		}
		pending[path] = b
		result.FilesDownloaded++
		result.BytesDownloaded += int64(len(b))
		result.Changed = true
	}
	for path, b := range pending {
		if err := writeAtomic(path, b); err != nil {
			return SyncResult{}, err
		}
	}
	if removeUnlisted(s.Dir, manifest.Files) > 0 {
		result.Changed = true
	}
	if err := writeAtomic(filepath.Join(s.Dir, "index.json"), index); err != nil {
		return SyncResult{}, err
	}
	meta.ETag, meta.CheckedAt, meta.SyncedAt = resp.Header.Get("ETag"), now, now
	if err := writeJSON(filepath.Join(s.Dir, "meta.json"), meta); err != nil {
		return SyncResult{}, err
	}
	s.clear()
	return result, nil
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("response is too large")
	}
	return b, nil
}
func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func sortedFiles(files map[string]FileInfo) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func removeUnlisted(dir string, files map[string]FileInfo) int {
	removed := 0
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "index.json" || rel == "meta.json" {
			return nil
		}
		if _, ok := files[filepath.ToSlash(rel)]; !ok && os.Remove(path) == nil {
			removed++
		}
		return nil
	})
	return removed
}
func (s *Store) clear() {
	s.manifest = nil
	s.posts = nil
	s.quiz = nil
	s.decks = nil
	s.checks = nil
	s.incidents = nil
	s.games = nil
	s.sponsors = nil
	s.bodies = nil
}

func (s *Store) Ensure(ctx context.Context, offline bool) (string, error) {
	s.mu.Lock()
	_, statErr := os.Stat(filepath.Join(s.Dir, "index.json"))
	meta, metaErr := s.readMeta()
	s.mu.Unlock()
	if metaErr != nil {
		return "", metaErr
	}
	if errors.Is(statErr, os.ErrNotExist) {
		if offline {
			return "", errors.New("No DevOps Daily content cached yet. Run devops-daily sync once while online.")
		}
		_, err := s.Sync(ctx)
		return "", err
	}
	if statErr != nil {
		return "", statErr
	}
	if offline || s.now().Sub(meta.CheckedAt) <= s.MaxAge {
		return "", nil
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if _, err := s.Sync(refreshCtx); err != nil {
		return fmt.Sprintf("Showing content from %s; could not refresh: %s", meta.SyncedAt.Local().Format("2006-01-02"), shortError(err)), nil
	}
	return "", nil
}

func shortError(err error) string {
	message := err.Error()
	if i := strings.IndexByte(message, '\n'); i >= 0 {
		message = message[:i]
	}
	return text.Line(message, 120)
}
