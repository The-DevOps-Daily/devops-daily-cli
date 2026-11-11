package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

const testIndexETag = "\"test-index-v1\""

type testServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests map[string]int
}

func (s *testServer) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[path]
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	files := testDataFiles(t)
	server := &testServer{requests: make(map[string]int)}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/cli/v1/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/cli/v1/")
		server.mu.Lock()
		server.requests[name]++
		server.mu.Unlock()
		if name == "index.json" && r.Header.Get("If-None-Match") == testIndexETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		body, ok := files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if name == "index.json" {
			w.Header().Set("ETag", testIndexETag)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func testDataFiles(t *testing.T) map[string][]byte {
	t.Helper()
	postBodies := []string{
		"# Pod OOMKilled\n\nA container exceeded its memory limit. [Learn more](https://devops-daily.com/kubernetes).",
		"# Terraform state lock\n\nA stale lock blocks your apply. [Learn more](https://devops-daily.com/terraform).",
		"# Docker layers\n\nLayers make \x1b[2Jbuilds repeatable. [Learn more](https://devops-daily.com/docker).",
	}
	posts := []map[string]any{
		{"slug": "pod-oomkilled", "title": "Why a pod was OOMKilled", "excerpt": "Inspect memory limits.", "date": "2026-11-10", "category": "Kubernetes", "tags": []string{"kubernetes", "memory"}, "readingTime": "4 min read", "url": "https://devops-daily.com/posts/pod-oomkilled", "body": "posts/0.json"},
		{"slug": "terraform-state-lock", "title": "Recover a Terraform state lock", "excerpt": "Unlock carefully.", "date": "2026-11-09", "category": "Terraform", "tags": []string{"terraform", "state"}, "readingTime": "5 min read", "url": "https://devops-daily.com/posts/terraform-state-lock", "body": "posts/1.json"},
		{"slug": "docker-layers", "title": "Understand \x1b[31mDocker\x1b[0m layers\x1b]2;owned\x07", "excerpt": "Build smaller images.", "date": "2026-11-08", "category": "Docker", "tags": []string{"docker", "images"}, "readingTime": "3 min read", "url": "https://devops-daily.com/posts/docker-layers", "body": "posts/2.json"},
	}
	files := map[string][]byte{
		"posts.json":      marshalTestJSON(t, map[string]any{"schema": 1, "posts": posts}),
		"quizzes.json":    marshalTestJSON(t, map[string]any{"schema": 1, "quizzes": []map[string]any{{"id": "docker-quiz", "title": "Docker quiz", "description": "Test Docker basics.", "category": "Docker", "url": "https://devops-daily.com/quizzes/docker-quiz", "questions": []map[string]any{{"id": "layers", "question": "What is a Docker layer?", "options": []string{"A", "B", "C", "D"}, "correct": 0, "explanation": "Layers are cached filesystem changes."}, {"id": "image", "question": "What is an image?", "options": []string{"A", "B", "C", "D"}, "correct": 1, "explanation": "An image is a template."}}}}}),
		"flashcards.json": marshalTestJSON(t, map[string]any{"schema": 1, "decks": []map[string]any{{"id": "docker-basics", "title": "Docker basics", "description": "Core Docker terms.", "category": "Docker", "difficulty": "beginner", "url": "https://devops-daily.com/flashcards/docker-basics", "cards": []map[string]any{{"id": "image", "front": "What is an image?", "back": "A packaged filesystem."}, {"id": "container", "front": "What is a container?", "back": "A running image."}}}}}),
		"checklists.json": marshalTestJSON(t, map[string]any{"schema": 1, "checklists": []map[string]any{{"id": "deploy-checklist", "title": "Deployment checklist", "description": "Ship safely.", "category": "Delivery", "difficulty": "beginner", "estimatedTime": "10 minutes", "url": "https://devops-daily.com/checklists/deploy-checklist", "items": []map[string]any{{"id": "health-check", "title": "Verify health checks", "description": "Check the service endpoint.", "critical": true}, {"id": "rollback", "title": "Prepare rollback", "description": "Keep a rollback path."}}}}}),
		"incidents.json":  marshalTestJSON(t, map[string]any{"schema": 1, "incidents": []map[string]any{{"id": "node-pressure", "title": "Node pressure incident", "topic": "Kubernetes", "difficulty": "intermediate", "technologies": []string{"Kubernetes"}, "scenario": "A node is under memory pressure and pods are being evicted.", "options": []map[string]string{{"key": "A", "text": "Ignore it"}, {"key": "B", "text": "Inspect the node"}, {"key": "C", "text": "Delete all pods"}, {"key": "D", "text": "Restart the cluster"}}, "correct": "B", "fix": "Inspect resource pressure and reschedule workloads.", "lesson": "Capacity signals need action.", "linkTitle": "Kubernetes guide", "linkUrl": "https://devops-daily.com/games/dns-simulator"}, {"id": "registry-outage", "title": "Registry outage incident", "topic": "Docker", "difficulty": "beginner", "technologies": []string{"Docker"}, "scenario": "Deployments cannot pull a required image.", "options": []map[string]string{{"key": "A", "text": "Retry forever"}, {"key": "B", "text": "Use a fallback registry"}, {"key": "C", "text": "Delete images"}, {"key": "D", "text": "Disable networking"}}, "correct": "B", "fix": "Use a mirrored image registry.", "lesson": "Critical dependencies need redundancy.", "linkTitle": "Docker guide", "linkUrl": "https://devops-daily.com/posts/docker-layers"}}}),
		"games.json":      marshalTestJSON(t, map[string]any{"schema": 1, "games": []map[string]string{{"id": "dns-simulator", "type": "simulator", "title": "DNS simulator", "description": "Trace DNS.", "category": "Networking", "url": "https://devops-daily.com/games/dns-simulator"}, {"id": "git-merge-conflict-simulator", "type": "simulator", "title": "Git merge conflict simulator", "description": "Resolve conflicts.", "category": "Git", "url": "https://devops-daily.com/games/git-merge-conflict-simulator"}, {"id": "opsville", "type": "game", "title": "Opsville", "description": "Run an operations team.", "category": "Operations", "url": "https://devops-daily.com/games/opsville"}}}),
		"sponsors.json":   marshalTestJSON(t, map[string]any{"schema": 1, "sponsors": []map[string]string{{"name": "Acme", "text": "Observability for teams.", "url": "https://go.devops-daily.com/acme?p=cli"}}}),
	}
	for i := 0; i < 16; i++ {
		bodies := map[string]string{}
		if i < len(posts) {
			bodies[posts[i]["slug"].(string)] = postBodies[i]
		} else {
			bodies[fmt.Sprintf("archived-post-%d", i)] = fmt.Sprintf("# Archived post %d\n\nAn archived post body. [Learn more](https://devops-daily.com/archive).", i)
		}
		files[fmt.Sprintf("posts/%d.json", i)] = marshalTestJSON(t, map[string]any{"schema": 1, "posts": bodies})
	}
	manifestFiles := make(map[string]map[string]any, len(files))
	for name, body := range files {
		sum := sha256.Sum256(body)
		manifestFiles[name] = map[string]any{"sha256": fmt.Sprintf("%x", sum), "bytes": len(body)}
	}
	files["index.json"] = marshalTestJSON(t, map[string]any{"schema": 1, "generatedAt": "2026-11-11T10:00:00Z", "site": "https://devops-daily.com", "files": manifestFiles, "counts": map[string]int{"posts": 3, "quizzes": 1, "flashcards": 1, "checklists": 1, "incidents": 2, "games": 3, "sponsors": 1}})
	return files
}

func marshalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

type testCalls struct {
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	mu          sync.Mutex
	urls        []string
	pagerTitles []string
	pagerBodies []string
	tuiModels   []tea.Model
	githubCode  int
	githubTag   string
}

func (c *testCalls) setGitHubResponse(code int, tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.githubCode = code
	c.githubTag = tag
}

func (c *testCalls) openedURLs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.urls...)
}

func (c *testCalls) pagerCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.pagerTitles...)
}

func (c *testCalls) tuiCalls() []tea.Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tea.Model(nil), c.tuiModels...)
}

type testGitHubTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t testGitHubTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copyRequest := request.Clone(request.Context())
	copyURL := *request.URL
	copyURL.Scheme = t.target.Scheme
	copyURL.Host = t.target.Host
	copyRequest.URL = &copyURL
	copyRequest.Host = ""
	return t.base.RoundTrip(copyRequest)
}

func newTestApp(t *testing.T, srv *testServer) (*App, *testCalls) {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("DEVOPS_DAILY_STATE_DIR", stateDir)
	calls := &testCalls{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, githubCode: http.StatusOK, githubTag: "v9.9.9"}
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.mu.Lock()
		code := calls.githubCode
		tag := calls.githubTag
		calls.mu.Unlock()
		if code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
	}))
	t.Cleanup(github.Close)
	githubURL, err := url.Parse(github.URL)
	if err != nil {
		t.Fatal(err)
	}
	githubClient := github.Client()
	githubTransport := githubClient.Transport
	if githubTransport == nil {
		githubTransport = http.DefaultTransport
	}
	githubClient.Transport = testGitHubTransport{target: githubURL, base: githubTransport}
	store := catalog.New(t.TempDir(), srv.URL+"/cli/v1/", "test")
	fixedNow := time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return fixedNow }
	app := &App{
		Stdout: calls.stdout,
		Stderr: calls.stderr,
		Stdin:  strings.NewReader(""),
		IsTTY:  func() bool { return false },
		Width:  func() int { return 100 },
		Now:    func() time.Time { return fixedNow },
		Store:  store,
		OpenURL: func(value string) error {
			calls.mu.Lock()
			defer calls.mu.Unlock()
			calls.urls = append(calls.urls, value)
			return nil
		},
		RunPager: func(title, body string) error {
			calls.mu.Lock()
			defer calls.mu.Unlock()
			calls.pagerTitles = append(calls.pagerTitles, title)
			calls.pagerBodies = append(calls.pagerBodies, body)
			return nil
		},
		RunTUI: func(model tea.Model) error {
			calls.mu.Lock()
			defer calls.mu.Unlock()
			calls.tuiModels = append(calls.tuiModels, model)
			return nil
		},
		HTTPClient: githubClient,
		Build:      BuildInfo{Version: "v1.2.3"},
	}
	return app, calls
}

func run(t *testing.T, app *App, args ...string) int {
	t.Helper()
	if output, ok := app.Stdout.(*bytes.Buffer); ok {
		output.Reset()
	}
	if output, ok := app.Stderr.(*bytes.Buffer); ok {
		output.Reset()
	}
	command := app.Command()
	command.SetArgs(args)
	if _, _, err := command.Find(args); err != nil {
		fmt.Fprintf(app.Stderr, "devops-daily: %s\n", err)
		fmt.Fprintln(app.Stderr, "Run 'devops-daily --help' for usage.")
		return 2
	}
	if err := command.Execute(); err != nil {
		fmt.Fprintf(app.Stderr, "devops-daily: %s\n", err)
		var use usageError
		if errors.As(err, &use) {
			fmt.Fprintln(app.Stderr, "Run 'devops-daily --help' for usage.")
			return 2
		}
		return 1
	}
	return 0
}
