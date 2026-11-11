package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncDownloadsThenUsesETag(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "sync"); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "Downloaded 23 files") {
		t.Fatalf("sync output = %q", calls.stdout.String())
	}
	if got := srv.count("index.json"); got != 1 {
		t.Fatalf("index requests = %d, want 1", got)
	}
	for _, name := range []string{"posts.json", "quizzes.json", "flashcards.json", "checklists.json", "incidents.json", "games.json", "sponsors.json"} {
		if got := srv.count(name); got != 1 {
			t.Fatalf("%s requests = %d, want 1", name, got)
		}
	}
	for _, name := range []string{"posts/0.json", "posts/1.json", "posts/2.json", "posts/3.json", "posts/4.json", "posts/5.json", "posts/6.json", "posts/7.json", "posts/8.json", "posts/9.json", "posts/10.json", "posts/11.json", "posts/12.json", "posts/13.json", "posts/14.json", "posts/15.json"} {
		if got := srv.count(name); got != 1 {
			t.Fatalf("%s requests = %d, want 1", name, got)
		}
	}
	if code := run(t, app, "sync"); code != 0 {
		t.Fatalf("second sync exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "Up to date.") {
		t.Fatalf("second sync output = %q", calls.stdout.String())
	}
	if got := srv.count("index.json"); got != 2 {
		t.Fatalf("index requests after second sync = %d, want 2", got)
	}
	for _, name := range []string{"posts.json", "quizzes.json", "flashcards.json", "checklists.json", "incidents.json", "games.json", "sponsors.json", "posts/0.json", "posts/15.json"} {
		if got := srv.count(name); got != 1 {
			t.Fatalf("%s requests after second sync = %d, want 1", name, got)
		}
	}
}

func TestPostsFiltersAndJSON(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "posts"); code != 0 {
		t.Fatalf("posts exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	for _, slug := range []string{"pod-oomkilled", "terraform-state-lock", "docker-layers"} {
		if !strings.Contains(calls.stdout.String(), slug) {
			t.Fatalf("posts output missing %q: %q", slug, calls.stdout.String())
		}
	}
	if code := run(t, app, "posts", "--tag", "k8s"); code != 0 {
		t.Fatalf("tagged posts exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "pod-oomkilled") || strings.Contains(calls.stdout.String(), "terraform-state-lock") || strings.Contains(calls.stdout.String(), "docker-layers") {
		t.Fatalf("tagged posts output = %q", calls.stdout.String())
	}
	if code := run(t, app, "posts", "--json"); code != 0 {
		t.Fatalf("JSON posts exit code = %d", code)
	}
	var posts []map[string]json.RawMessage
	if err := json.Unmarshal(calls.stdout.Bytes(), &posts); err != nil {
		t.Fatalf("posts JSON: %v\n%s", err, calls.stdout.String())
	}
	if len(posts) != 3 {
		t.Fatalf("JSON post count = %d, want 3", len(posts))
	}
	for _, post := range posts {
		if _, ok := post["slug"]; !ok {
			t.Fatalf("post has no slug: %s", calls.stdout.String())
		}
		if _, ok := post["body"]; ok {
			t.Fatalf("post unexpectedly has body: %s", calls.stdout.String())
		}
	}
	if code := run(t, app, "posts", "--tag", "unknown"); code != 0 {
		t.Fatalf("unknown tag exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "No posts for tag") {
		t.Fatalf("unknown tag output = %q", calls.stdout.String())
	}
}

func TestReadExactPrefixAndErrors(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "read", "pod-oomkilled"); code != 0 {
		t.Fatalf("exact read exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if len(calls.pagerCalls()) != 0 {
		t.Fatalf("pager called for non-terminal read: %v", calls.pagerCalls())
	}
	if !strings.Contains(calls.stdout.String(), "Why a pod was OOMKilled") || !strings.Contains(calls.stdout.String(), "Read on the site:") {
		t.Fatalf("exact read output = %q", calls.stdout.String())
	}
	if strings.Contains(calls.stdout.String(), "\x1b") {
		t.Fatalf("non-terminal read contains ANSI escape: %q", calls.stdout.String())
	}
	if code := run(t, app, "read", "terraform"); code != 0 {
		t.Fatalf("prefix read exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "Recover a Terraform state lock") {
		t.Fatalf("prefix read output = %q", calls.stdout.String())
	}
	if code := run(t, app, "read", "nope"); code != 1 {
		t.Fatalf("missing read exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stderr.String(), "No post called") {
		t.Fatalf("missing read stderr = %q", calls.stderr.String())
	}
}

func TestReadUsesPagerOnTerminal(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	app.IsTTY = func() bool { return true }
	if code := run(t, app, "read", "pod-oomkilled"); code != 0 {
		t.Fatalf("terminal read exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	pagers := calls.pagerCalls()
	if len(pagers) != 1 || pagers[0] != "Why a pod was OOMKilled" {
		t.Fatalf("pager calls = %v", pagers)
	}
}

func TestPlayAndSubscribe(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "play"); code != 0 {
		t.Fatalf("play list exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "dns-simulator") || !strings.Contains(calls.stdout.String(), "opsville") {
		t.Fatalf("play list output = %q", calls.stdout.String())
	}
	if code := run(t, app, "play", "dns", "--print"); code != 0 {
		t.Fatalf("play print exit code = %d", code)
	}
	if got, want := calls.stdout.String(), "https://devops-daily.com/games/dns-simulator\n"; got != want {
		t.Fatalf("play print output = %q, want %q", got, want)
	}
	if code := run(t, app, "play", "dns"); code != 0 {
		t.Fatalf("play open exit code = %d", code)
	}
	urls := calls.openedURLs()
	if len(urls) != 1 || urls[0] != "https://devops-daily.com/games/dns-simulator" {
		t.Fatalf("opened URLs = %v", urls)
	}
	if code := run(t, app, "play", "simulator"); code != 1 {
		t.Fatalf("ambiguous play exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if code := run(t, app, "play", "zzz"); code != 1 {
		t.Fatalf("missing play exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if code := run(t, app, "subscribe", "--print"); code != 0 {
		t.Fatalf("subscribe print exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "https://devops-daily.com/newsletters") {
		t.Fatalf("subscribe print output = %q", calls.stdout.String())
	}
}

func TestIncidentAndVersion(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "incident"); code != 0 {
		t.Fatalf("incident exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "Node pressure incident") && !strings.Contains(calls.stdout.String(), "Registry outage incident") {
		t.Fatalf("incident output has no fixture title: %q", calls.stdout.String())
	}
	if !strings.Contains(calls.stdout.String(), "Run with --fix") {
		t.Fatalf("incident output = %q", calls.stdout.String())
	}
	if code := run(t, app, "incident", "--fix"); code != 0 {
		t.Fatalf("incident fix exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "The fix:") || !strings.Contains(calls.stdout.String(), "https://devops-daily.com/") {
		t.Fatalf("incident fix output = %q", calls.stdout.String())
	}
	app.IsTTY = func() bool { return true }
	if code := run(t, app, "incident"); code != 0 {
		t.Fatalf("terminal incident exit code = %d", code)
	}
	if len(calls.tuiCalls()) != 1 || calls.tuiCalls()[0] == nil {
		t.Fatalf("TUI models = %v", calls.tuiCalls())
	}
	if code := run(t, app, "version"); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "devops-daily") || !strings.Contains(calls.stdout.String(), "v1.2.3") {
		t.Fatalf("version output = %q", calls.stdout.String())
	}
}

func TestUpdateAndUpgradeMethod(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	calls.setGitHubResponse(200, "v9.9.9")
	if code := run(t, app, "update"); code != 0 {
		t.Fatalf("update exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "Version v9.9.9 is out") {
		t.Fatalf("new release output = %q", calls.stdout.String())
	}
	calls.setGitHubResponse(200, "v1.2.3")
	if code := run(t, app, "update"); code != 0 {
		t.Fatalf("same-version update exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "latest") {
		t.Fatalf("same-version output = %q", calls.stdout.String())
	}
	calls.setGitHubResponse(404, "")
	if code := run(t, app, "update"); code != 0 {
		t.Fatalf("missing-release update exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "No release is published yet.") {
		t.Fatalf("missing-release output = %q", calls.stdout.String())
	}
	app.Build.Version = "dev"
	if code := run(t, app, "update"); code != 0 {
		t.Fatalf("development update exit code = %d", code)
	}
	if !strings.Contains(calls.stdout.String(), "development build") {
		t.Fatalf("development output = %q", calls.stdout.String())
	}
	if got := UpgradeMethod("/usr/local/Cellar/devops-daily/1.0/bin/devops-daily"); got != "brew upgrade devops-daily" {
		t.Fatalf("Homebrew upgrade method = %q", got)
	}
	if got := UpgradeMethod("/usr/local/lib/node_modules/@devopsdaily/devops-daily-cli/bin/devops-daily"); got != "npm install -g @devopsdaily/devops-daily-cli@latest" {
		t.Fatalf("npm upgrade method = %q", got)
	}
	goBin := t.TempDir()
	t.Setenv("GOBIN", goBin)
	if got := UpgradeMethod(filepath.Join(goBin, "devops-daily")); got != "go install github.com/The-DevOps-Daily/devops-daily-cli/cmd/devops-daily@latest" {
		t.Fatalf("GOBIN upgrade method = %q", got)
	}
	if got := UpgradeMethod("/opt/devops-daily"); !strings.Contains(got, "install.sh") {
		t.Fatalf("fallback upgrade method = %q", got)
	}
}

func TestOfflineAndUsageFailures(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "--offline", "posts"); code != 1 {
		t.Fatalf("offline posts exit code = %d", code)
	}
	if !strings.Contains(calls.stderr.String(), "No DevOps Daily content cached yet") {
		t.Fatalf("offline posts stderr = %q", calls.stderr.String())
	}
	if code := run(t, app, "not-a-command"); code != 2 {
		t.Fatalf("unknown command exit code = %d", code)
	}
	if code := run(t, app, "posts", "--not-a-flag"); code != 2 {
		t.Fatalf("unknown flag exit code = %d", code)
	}
}

func TestDataURLAndNoColor(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	t.Setenv("DEVOPS_DAILY_DATA_URL", "ftp://x")
	if code := run(t, app, "posts"); code != 1 {
		t.Fatalf("invalid data URL exit code = %d", code)
	}
	if !strings.Contains(calls.stderr.String(), "must be an https:// URL") {
		t.Fatalf("invalid data URL stderr = %q", calls.stderr.String())
	}
	t.Setenv("DEVOPS_DAILY_DATA_URL", "")
	if code := run(t, app, "--no-color", "posts"); code != 0 {
		t.Fatalf("no-color posts exit code = %d", code)
	}
	if strings.Contains(calls.stdout.String(), "\x1b") {
		t.Fatalf("no-color posts output contains ANSI escape: %q", calls.stdout.String())
	}
}
