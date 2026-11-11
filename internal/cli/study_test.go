package cli

import (
	"strings"
	"testing"
)

func TestQuizListsAndStartsTUI(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "quiz"); code != 0 {
		t.Fatalf("quiz list exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "docker-quiz") {
		t.Fatalf("quiz list output = %q", calls.stdout.String())
	}
	if code := run(t, app, "quiz", "docker"); code != 1 {
		t.Fatalf("non-terminal quiz exit code = %d", code)
	}
	if !strings.Contains(calls.stderr.String(), "interactive terminal") {
		t.Fatalf("non-terminal quiz stderr = %q", calls.stderr.String())
	}
	app.IsTTY = func() bool { return true }
	if code := run(t, app, "quiz", "docker"); code != 0 {
		t.Fatalf("terminal quiz exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	models := calls.tuiCalls()
	if len(models) != 1 || models[0] == nil {
		t.Fatalf("quiz TUI models = %v", models)
	}
}

func TestFlashcardsAndChecklistListing(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "flashcards"); code != 0 {
		t.Fatalf("flashcards list exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "docker-basics") || !strings.Contains(calls.stdout.String(), "2 cards") {
		t.Fatalf("flashcards output = %q", calls.stdout.String())
	}
	if code := run(t, app, "checklist"); code != 0 {
		t.Fatalf("checklist list exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	if !strings.Contains(calls.stdout.String(), "deploy-checklist") || !strings.Contains(calls.stdout.String(), "0/2") {
		t.Fatalf("checklist list output = %q", calls.stdout.String())
	}
}

func TestChecklistPlainAndReset(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "checklist", "deploy-checklist"); code != 0 {
		t.Fatalf("plain checklist exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	output := calls.stdout.String()
	if !strings.Contains(output, "[ ] Verify health checks (critical)") || !strings.Contains(output, "[ ] Prepare rollback") {
		t.Fatalf("plain checklist output = %q", output)
	}
	if code := run(t, app, "checklist", "deploy-checklist", "--reset"); code != 1 {
		t.Fatalf("unconfirmed reset exit code = %d", code)
	}
	if !strings.Contains(calls.stderr.String(), "--reset needs --yes") {
		t.Fatalf("unconfirmed reset stderr = %q", calls.stderr.String())
	}
	if code := run(t, app, "checklist", "deploy-checklist", "--reset", "--yes"); code != 0 {
		t.Fatalf("confirmed reset exit code = %d, stderr = %s", code, calls.stderr.String())
	}
}
