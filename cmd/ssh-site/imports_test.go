package main

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The check commands connect to any host they are given. In the SSH app
// that would let a visitor make our server connect anywhere, so the server
// must not be able to reach them, not even through another package.
func TestSSHAppCannotReachCheckCommands(t *testing.T) {
	root, module := moduleRoot(t)
	deps := map[string]bool{}
	walkImports(t, root, module, module+"/cmd/ssh-site", deps)
	if !deps[module+"/internal/tui"] {
		t.Fatalf("import walk found no internal/tui, so it is broken: %v", deps)
	}
	for _, banned := range []string{module + "/internal/check", module + "/internal/cli"} {
		if deps[banned] {
			t.Errorf("cmd/ssh-site imports %s; the network checks must stay out of the SSH app", banned)
		}
	}
}

func moduleRoot(t *testing.T) (string, string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if file, err := os.Open(filepath.Join(dir, "go.mod")); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				if module, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
					return dir, strings.TrimSpace(module)
				}
			}
			t.Fatal("go.mod has no module line")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// walkImports records every package of this module that pkg imports,
// directly or not. Test files do not count; files for every OS do.
func walkImports(t *testing.T, root, module, pkg string, seen map[string]bool) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkg, module)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if (path == module || strings.HasPrefix(path, module+"/")) && !seen[path] {
				seen[path] = true
				walkImports(t, root, module, path, seen)
			}
		}
	}
}
