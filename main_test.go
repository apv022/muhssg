package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("muhssg %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func TestCLIWorkflow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-site")
	runCLI(t, "new", "site", dir, "--title", "My: Site")
	cfg, err := os.ReadFile(filepath.Join(dir, "muhssg.yaml"))
	if err != nil || !strings.Contains(string(cfg), `title: "My: Site"`) {
		t.Fatalf("title not quoted in config: %v", err)
	}

	out := runCLI(t, "new", "writing", "Second Post!", "--root", dir)
	if !strings.Contains(out, "second-post.md") {
		t.Fatalf("new post output: %s", out)
	}
	post, _ := os.ReadFile(filepath.Join(dir, "content/writing/second-post.md"))
	if !strings.Contains(string(post), "draft: true") || !strings.Contains(string(post), `title: "Second Post!"`) {
		t.Fatalf("new post content:\n%s", post)
	}
	if err := run([]string{"new", "writing", "Second Post!", "--root", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("creating the same post twice should fail")
	}

	out = runCLI(t, "build", "--root", dir)
	if !strings.Contains(out, "✓ built") {
		t.Fatalf("build output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist/content")); err == nil {
		t.Fatal("unexpected folder in output")
	}
	if !strings.Contains(runCLI(t, "check", "--root", dir), "no broken links") {
		t.Fatal("check failed")
	}
}

func TestCLIMisc(t *testing.T) {
	if !strings.Contains(runCLI(t, "version"), "muhssg") {
		t.Fatal("version")
	}
	if !strings.Contains(runCLI(t, "styles"), "monokai") {
		t.Fatal("styles")
	}
	css := runCLI(t, "syntax", "github", "--dark", "github-dark")
	if !strings.Contains(css, "prefers-color-scheme: dark") {
		t.Fatal("syntax --dark")
	}
	if err := run([]string{"syntax", "no-such-style"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown style should fail")
	}
	if err := run([]string{"frobnicate"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown command should fail")
	}
	if err := run([]string{"new", "site", t.TempDir() + "/x", "extra"}, &bytes.Buffer{}); err == nil {
		t.Fatal("extra args should fail")
	}
}

func TestNewSiteRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644)
	if err := run([]string{"new", "site", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("should refuse a non-empty folder")
	}
}
