package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPrintErrorStripsEscapeSequences(t *testing.T) {
	var out bytes.Buffer
	printError(&out, errors.New("download index.json: \x1b[2J\x1b]2;owned\x07boom\nnext line"))
	if got, want := out.String(), "devops-daily: download index.json: boom\nnext line\n"; got != want {
		t.Fatalf("printError wrote %q, want %q", got, want)
	}
}

func TestRemoteTitleAndBodyAreSanitised(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	if code := run(t, app, "posts"); code != 0 {
		t.Fatalf("posts exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	posts := calls.stdout.String()
	if strings.ContainsAny(posts, "\x1b\x07") || strings.Contains(posts, "owned") || strings.Contains(posts, "[31m") {
		t.Fatalf("posts output carries escape sequences: %q", posts)
	}
	if !strings.Contains(posts, "Understand Docker layers") {
		t.Fatalf("posts output lost the cleaned title: %q", posts)
	}
	calls.stdout.Reset()
	if code := run(t, app, "read", "docker-layers", "--no-pager"); code != 0 {
		t.Fatalf("read exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	article := calls.stdout.String()
	if strings.ContainsAny(article, "\x1b\x07") || strings.Contains(article, "[2J") {
		t.Fatalf("read output carries escape sequences: %q", article)
	}
	if !strings.Contains(article, "Layers make builds repeatable") {
		t.Fatalf("read output lost the body text: %q", article)
	}
}
