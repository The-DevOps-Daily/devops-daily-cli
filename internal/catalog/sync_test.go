package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type catalogTestServer struct {
	*httptest.Server
	files       map[string][]byte
	etag        string
	indexStatus int
	requests    map[string]int
}

func newCatalogTestServer(t *testing.T, files map[string][]byte, etag string) *catalogTestServer {
	t.Helper()
	server := &catalogTestServer{requests: make(map[string]int)}
	server.setFiles(t, files, etag)
	server.Server = httptest.NewServer(http.HandlerFunc(server.serveHTTP))
	t.Cleanup(server.Close)
	return server
}

func (s *catalogTestServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	s.requests[name]++
	if name == "index.json" {
		if s.indexStatus != 0 {
			w.WriteHeader(s.indexStatus)
			return
		}
		if s.etag != "" && r.Header.Get("If-None-Match") == s.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", s.etag)
	}
	body, ok := s.files[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *catalogTestServer) setFiles(t *testing.T, files map[string][]byte, etag string) {
	t.Helper()
	s.files = catalogCopyFiles(files)
	s.files["index.json"] = catalogManifest(t, files)
	s.etag = etag
	s.indexStatus = 0
}

func (s *catalogTestServer) count(name string) int {
	return s.requests[name]
}

func (s *catalogTestServer) totalRequests() int {
	total := 0
	for _, count := range s.requests {
		total += count
	}
	return total
}

func catalogCopyFiles(files map[string][]byte) map[string][]byte {
	copy := make(map[string][]byte, len(files))
	for name, body := range files {
		copy[name] = append([]byte(nil), body...)
	}
	return copy
}

func catalogJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func catalogManifest(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	entries := make(map[string]FileInfo, len(files))
	for name, body := range files {
		entries[name] = FileInfo{SHA256: catalogSHA256(body), Bytes: int64(len(body))}
	}
	return catalogJSON(t, Manifest{
		Schema:      Schema,
		GeneratedAt: "2026-11-11T10:00:00Z",
		Site:        "https://devops-daily.com",
		Files:       entries,
	})
}

func catalogSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return stringAppendHex(sum[:])
}

func stringAppendHex(value []byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(value)*2)
	for i, b := range value {
		encoded[i*2] = digits[b>>4]
		encoded[i*2+1] = digits[b&0x0f]
	}
	return string(encoded)
}

func catalogFixtureFiles(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"posts.json": catalogJSON(t, map[string]any{
			"schema": Schema,
			"posts": []map[string]any{{
				"slug":  "hello",
				"title": "Hello",
				"date":  "2026-11-11",
				"url":   "https://devops-daily.com/posts/hello",
				"body":  "posts/0.json",
			}},
		}),
		"posts/0.json": catalogJSON(t, map[string]any{
			"schema": Schema,
			"posts":  map[string]string{"hello": "Hello body"},
		}),
		"quizzes.json": catalogJSON(t, map[string]any{
			"schema": Schema,
			"quizzes": []map[string]any{{
				"id":    "quiz",
				"title": "Quiz",
				"url":   "https://devops-daily.com/quizzes/quiz",
				"questions": []map[string]any{{
					"id":       "question",
					"question": "Question?",
					"options":  []string{"one"},
					"correct":  0,
				}},
			}},
		}),
		"games.json": catalogJSON(t, map[string]any{
			"schema": Schema,
			"games": []map[string]any{{
				"id":    "game",
				"type":  "game",
				"title": "Game",
				"url":   "https://devops-daily.com/games/game",
			}},
		}),
	}
}

func newCatalogStore(dir string, server *catalogTestServer) *Store {
	return New(dir, server.URL+"/", "catalog-test")
}

func catalogReadMeta(t *testing.T, dir string) cacheMeta {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta cacheMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

func catalogWriteFile(t *testing.T, dir, name string, body []byte) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyncDownloadsManifestFiles(t *testing.T) {
	files := catalogFixtureFiles(t)
	server := newCatalogTestServer(t, files, "\"v1\"")
	dir := t.TempDir()
	store := newCatalogStore(dir, server)

	result, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var bytes int64
	for name, body := range files {
		bytes += int64(len(body))
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatalf("cached %s: %v", name, err)
		}
	}
	if result.FilesDownloaded != len(files) {
		t.Fatalf("FilesDownloaded = %d, want %d", result.FilesDownloaded, len(files))
	}
	if result.BytesDownloaded != bytes {
		t.Fatalf("BytesDownloaded = %d, want %d", result.BytesDownloaded, bytes)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	for _, name := range []string{"index.json", "meta.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestSyncNotModifiedUpdatesCheckedAt(t *testing.T) {
	server := newCatalogTestServer(t, catalogFixtureFiles(t), "\"v1\"")
	store := newCatalogStore(t.TempDir(), server)
	now := time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time {
		return now
	}
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := catalogReadMeta(t, store.Dir)
	now = now.Add(time.Hour)
	result, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	after := catalogReadMeta(t, store.Dir)
	if result != (SyncResult{}) {
		t.Fatalf("SyncResult = %#v, want zero result", result)
	}
	if !after.CheckedAt.After(before.CheckedAt) {
		t.Fatalf("CheckedAt = %s, want after %s", after.CheckedAt, before.CheckedAt)
	}
	if server.count("index.json") != 2 {
		t.Fatalf("index requests = %d, want 2", server.count("index.json"))
	}
	for name := range server.files {
		if name == "index.json" {
			continue
		}
		if server.count(name) != 1 {
			t.Fatalf("%s requests = %d, want 1", name, server.count(name))
		}
	}
}

func TestSyncDownloadsOnlyChangedFile(t *testing.T) {
	files := catalogFixtureFiles(t)
	server := newCatalogTestServer(t, files, "\"v1\"")
	store := newCatalogStore(t.TempDir(), server)
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	files["posts.json"] = catalogJSON(t, map[string]any{
		"schema": Schema,
		"posts": []map[string]any{{
			"slug":  "hello",
			"title": "Changed title",
			"date":  "2026-11-11",
			"url":   "https://devops-daily.com/posts/hello",
			"body":  "posts/0.json",
		}},
	})
	server.setFiles(t, files, "\"v2\"")

	result, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesDownloaded != 1 {
		t.Fatalf("FilesDownloaded = %d, want 1", result.FilesDownloaded)
	}
	if result.BytesDownloaded != int64(len(files["posts.json"])) {
		t.Fatalf("BytesDownloaded = %d, want %d", result.BytesDownloaded, len(files["posts.json"]))
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if server.count("posts.json") != 2 {
		t.Fatalf("posts.json requests = %d, want 2", server.count("posts.json"))
	}
	for name := range files {
		if name == "posts.json" {
			continue
		}
		if server.count(name) != 1 {
			t.Fatalf("%s requests = %d, want 1", name, server.count(name))
		}
	}
}

func TestSyncRejectsHashMismatchWithoutReplacingCache(t *testing.T) {
	files := catalogFixtureFiles(t)
	server := newCatalogTestServer(t, files, "\"v1\"")
	store := newCatalogStore(t.TempDir(), server)
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldFile, err := os.ReadFile(filepath.Join(store.Dir, "posts.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldIndex, err := os.ReadFile(filepath.Join(store.Dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	expectedFiles := catalogCopyFiles(files)
	expectedFiles["posts.json"] = []byte(`{"schema":1,"posts":[]}`)
	server.setFiles(t, expectedFiles, "\"v2\"")
	server.files["posts.json"] = []byte(`{"schema":1,"posts":[`)

	if _, err := store.Sync(context.Background()); err == nil {
		t.Fatal("Sync succeeded with a hash mismatch")
	}
	gotFile, err := os.ReadFile(filepath.Join(store.Dir, "posts.json"))
	if err != nil {
		t.Fatal(err)
	}
	gotIndex, err := os.ReadFile(filepath.Join(store.Dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotFile) != string(oldFile) {
		t.Fatal("cached file changed after a failed sync")
	}
	if string(gotIndex) != string(oldIndex) {
		t.Fatal("index.json changed after a failed sync")
	}
}

func TestSyncRejectsBadManifestFileNames(t *testing.T) {
	for _, name := range []string{"../x.json", "/etc/passwd.json", "a/b/c.json", "UPPER.json"} {
		t.Run(name, func(t *testing.T) {
			server := newCatalogTestServer(t, nil, "\"bad\"")
			server.files["index.json"] = catalogJSON(t, Manifest{
				Schema: Schema,
				Files: map[string]FileInfo{name: {
					SHA256: catalogSHA256([]byte("ignored")),
					Bytes:  7,
				}},
			})
			dir := t.TempDir()
			store := newCatalogStore(dir, server)

			if _, err := store.Sync(context.Background()); err == nil {
				t.Fatal("Sync succeeded with an invalid file name")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("cache entries = %d, want 0", len(entries))
			}
			if server.count(name) != 0 {
				t.Fatalf("%s requests = %d, want 0", name, server.count(name))
			}
			if name == "../x.json" {
				outside := filepath.Join(filepath.Dir(dir), "x.json")
				if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("outside cache file %s was created", outside)
				}
			}
		})
	}
}

func TestSyncRejectsNewerSchema(t *testing.T) {
	server := newCatalogTestServer(t, nil, "\"v2\"")
	server.files["index.json"] = catalogJSON(t, Manifest{Schema: 2})
	store := newCatalogStore(t.TempDir(), server)

	_, err := store.Sync(context.Background())
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("Sync error = %v, want ErrNewerSchema", err)
	}
}

func TestSyncRefusesFileLargerThanManifest(t *testing.T) {
	files := map[string][]byte{"posts.json": []byte(`{"schema":1,"posts":[]}`)}
	server := newCatalogTestServer(t, files, "\"v1\"")
	manifest := Manifest{
		Schema: Schema,
		Files: map[string]FileInfo{
			"posts.json": {
				SHA256: catalogSHA256(files["posts.json"]),
				Bytes:  int64(len(files["posts.json"]) - 1),
			},
		},
	}
	server.files["index.json"] = catalogJSON(t, manifest)
	store := newCatalogStore(t.TempDir(), server)

	if _, err := store.Sync(context.Background()); err == nil {
		t.Fatal("Sync succeeded with a response larger than its manifest entry")
	}
}

func TestEnsureOfflineUsesCacheWithoutRequest(t *testing.T) {
	server := newCatalogTestServer(t, catalogFixtureFiles(t), "\"v1\"")
	empty := newCatalogStore(t.TempDir(), server)
	if _, err := empty.Ensure(context.Background(), true); err == nil || !strings.Contains(err.Error(), "No DevOps Daily content cached yet") {
		t.Fatalf("offline empty-cache error = %v", err)
	}
	if server.totalRequests() != 0 {
		t.Fatalf("empty offline requests = %d, want 0", server.totalRequests())
	}

	store := newCatalogStore(t.TempDir(), server)
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests := server.totalRequests()
	notice, err := store.Ensure(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if notice != "" {
		t.Fatalf("notice = %q, want empty", notice)
	}
	if server.totalRequests() != requests {
		t.Fatalf("offline requests = %d, want %d", server.totalRequests(), requests)
	}
}

func TestEnsureStaleCacheShowsNoticeAndKeepsData(t *testing.T) {
	server := newCatalogTestServer(t, catalogFixtureFiles(t), "\"v1\"")
	store := newCatalogStore(t.TempDir(), server)
	now := time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time {
		return now
	}
	store.MaxAge = time.Hour
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	server.indexStatus = http.StatusInternalServerError

	notice, err := store.Ensure(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(notice, "Showing content from") {
		t.Fatalf("notice = %q, want stale-content notice", notice)
	}
	posts, err := store.Posts()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].Slug != "hello" {
		t.Fatalf("Posts = %#v, want cached hello post", posts)
	}
}

func TestLoadersCleanTextAndURLs(t *testing.T) {
	dir := t.TempDir()
	catalogWriteFile(t, dir, "posts.json", catalogJSON(t, map[string]any{
		"schema": Schema,
		"posts": []map[string]any{{
			"slug":  "hello",
			"title": "Hello \x1b[31mred\x1b[0m",
			"date":  "2026-11-11",
			"url":   "https://devops-daily.com/posts/hello",
			"body":  "posts/0.json",
		}},
	}))
	catalogWriteFile(t, dir, "quizzes.json", catalogJSON(t, map[string]any{
		"schema": Schema,
		"quizzes": []map[string]any{{
			"id":    "quiz",
			"title": "Quiz",
			"url":   "https://devops-daily.com/quizzes/quiz",
			"questions": []map[string]any{{
				"id":          "question",
				"question":    "Question?",
				"options":     []string{"one"},
				"correct":     0,
				"explanation": "Bell\x07removed",
			}},
		}},
	}))
	catalogWriteFile(t, dir, "games.json", catalogJSON(t, map[string]any{
		"schema": Schema,
		"games": []map[string]any{
			{"id": "http", "type": "game", "title": "HTTP", "url": "http://example.com"},
			{"id": "script", "type": "game", "title": "Script", "url": "javascript:alert(1)"},
		},
	}))
	store := New(dir, "", "")

	posts, err := store.Posts()
	if err != nil {
		t.Fatal(err)
	}
	quizzes, err := store.Quizzes()
	if err != nil {
		t.Fatal(err)
	}
	games, err := store.Games()
	if err != nil {
		t.Fatal(err)
	}
	catalogNoControls(t, posts[0].Title)
	catalogNoControls(t, quizzes[0].Questions[0].Explanation)
	if games[0].URL != "" {
		t.Fatalf("http game URL = %q, want empty", games[0].URL)
	}
	if games[1].URL != "" {
		t.Fatalf("javascript game URL = %q, want empty", games[1].URL)
	}
}

func catalogNoControls(t *testing.T, value string) {
	t.Helper()
	for _, b := range []byte(value) {
		if b < 0x20 && b != '\n' {
			t.Fatalf("%q contains control byte 0x%02x", value, b)
		}
	}
}

func TestSyncRemovesFilesAbsentFromNewManifest(t *testing.T) {
	files := catalogFixtureFiles(t)
	server := newCatalogTestServer(t, files, "\"v1\"")
	store := newCatalogStore(t.TempDir(), server)
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	delete(files, "quizzes.json")
	server.setFiles(t, files, "\"v2\"")

	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "quizzes.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quizzes.json stat error = %v, want not exist", err)
	}
}

func TestPostBodyReadsShardAndRejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	catalogWriteFile(t, dir, "posts/0.json", catalogJSON(t, map[string]any{
		"schema": Schema,
		"posts":  map[string]string{"hello": "Hello body"},
	}))
	store := New(dir, "", "")
	meta := PostMeta{Slug: "hello", Body: "posts/0.json"}

	body, err := store.PostBody(meta)
	if err != nil {
		t.Fatal(err)
	}
	if body != "Hello body" {
		t.Fatalf("PostBody = %q, want %q", body, "Hello body")
	}
	missing := meta
	missing.Slug = "missing"
	if _, err := store.PostBody(missing); err == nil {
		t.Fatal("PostBody succeeded for a missing slug")
	}
	unsafe := meta
	unsafe.Body = "../posts.json"
	if _, err := store.PostBody(unsafe); err == nil {
		t.Fatal("PostBody accepted an unsafe body file name")
	}
}
