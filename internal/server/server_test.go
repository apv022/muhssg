package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeFile(t *testing.T) {
	out := t.TempDir()
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(filepath.Join(out, p)), 0o755)
		os.WriteFile(filepath.Join(out, p), []byte(s), 0o644)
	}
	write("index.html", "<html><body>home</body></html>")
	write("about/index.html", "<html><body>about</body></html>")
	write("404.html", "<html><body>missing</body></html>")
	write("_assets/site.css", "body{}")

	get := func(path, base string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		serveFile(rec, httptest.NewRequest(http.MethodGet, path, nil), out, base)
		return rec
	}

	if r := get("/", ""); r.Code != 200 || !strings.Contains(r.Body.String(), eventsPath) {
		t.Fatalf("home: %d %q", r.Code, r.Body.String())
	}
	if r := get("/about", ""); r.Code != http.StatusMovedPermanently || r.Header().Get("Location") != "/about/" {
		t.Fatalf("about without slash: %d %v", r.Code, r.Header())
	}
	if r := get("/about/", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "about") {
		t.Fatalf("about: %d", r.Code)
	}
	if r := get("/nope/", ""); r.Code != 404 || !strings.Contains(r.Body.String(), "missing") {
		t.Fatalf("404: %d %q", r.Code, r.Body.String())
	}
	if r := get("/_assets/site.css", ""); r.Code != 200 || strings.Contains(r.Body.String(), eventsPath) {
		t.Fatalf("css: %d", r.Code)
	}
	if r := get("/../../etc/passwd", ""); r.Code != 404 {
		t.Fatalf("path traversal: %d", r.Code)
	}
	if r := get("/blog/about/", "/blog"); r.Code != 200 {
		t.Fatalf("base path: %d", r.Code)
	}
	if r := get("/about/", "/blog"); r.Code != 404 {
		t.Fatalf("outside base path: %d", r.Code)
	}
}
