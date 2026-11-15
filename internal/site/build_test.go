package site_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/apv022/muhssg/internal/scaffold"
	"github.com/apv022/muhssg/internal/site"
)

// starter creates a fresh copy of the starter site.
func starter(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "site")
	if err := scaffold.NewSite(dir, "Test Site"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func build(t *testing.T, root string, opts site.Options) (*site.Result, string) {
	t.Helper()
	opts.Root = root
	res, err := site.Build(opts)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return res, res.OutDir
}

// read returns a file's contents with line endings normalised to \n, so
// tests that edit text behave the same on Windows checkouts.
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// testPNG returns a w×h PNG as a string.
func testPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func mustContain(t *testing.T, label, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s: missing %q", label, w)
		}
	}
}

func mustNotContain(t *testing.T, label, s string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(s, b) {
			t.Errorf("%s: should not contain %q", label, b)
		}
	}
}

func TestStarterSiteBuilds(t *testing.T) {
	root := starter(t)
	res, out := build(t, root, site.Options{})
	if len(res.Warnings) != 0 {
		t.Errorf("warnings: %v", res.Warnings)
	}
	for _, f := range []string{
		"index.html", "about/index.html", "404.html", "writing/index.html",
		"writing/hello-world/index.html", "writing/rss.xml", "sitemap.xml", "robots.txt",
		"tags/index.html", "tags/muhssg/index.html", "blog/index.html", "_redirects",
		"favicon.ico", "apple-touch-icon.png", "icon-192.png", "icon-512.png", "site.webmanifest",
	} {
		if !exists(filepath.Join(out, f)) {
			t.Errorf("missing %s", f)
		}
	}
	if exists(filepath.Join(out, "writing/unfinished/index.html")) {
		t.Error("draft was built")
	}

	post := read(t, filepath.Join(out, "writing/hello-world/index.html"))
	mustContain(t, "post", post,
		"<title>Hello, world — Test Site</title>",
		`<link rel="canonical" href="https://example.com/writing/hello-world/">`,
		`<meta property="og:type" content="article">`,
		`<link rel="alternate" type="application/rss+xml" title="Writing" href="/writing/rss.xml">`,
		`<link rel="icon" href="/favicon.ico"`,
		`class="chroma"`,
		`<a href="#code">Code</a>`,
		`aria-current="page"`,
	)
	if !regexp.MustCompile(`<link rel="stylesheet" href="/_assets/site-[A-Z0-9]+\.css">`).MatchString(post) {
		t.Error("post does not link a hashed site.css")
	}
	if !regexp.MustCompile(`<script type="module" src="/_assets/app-[A-Z0-9]+\.js"></script>`).MatchString(post) {
		t.Error("post does not load a hashed app.js")
	}

	// CSS @imports are bundled, TypeScript is compiled.
	matches, _ := filepath.Glob(filepath.Join(out, "_assets", "site-*.css"))
	if len(matches) != 1 {
		t.Fatalf("site css files: %v", matches)
	}
	css := read(t, matches[0])
	mustContain(t, "site.css", css, "--bg:")
	mustNotContain(t, "site.css", css, "@import")
	js, _ := filepath.Glob(filepath.Join(out, "_assets", "app-*.js"))
	if len(js) != 1 || strings.Contains(read(t, js[0]), ": string") {
		t.Errorf("app.ts not compiled: %v", js)
	}
	syntax, _ := filepath.Glob(filepath.Join(out, "_assets", "syntax-*.css"))
	if len(syntax) != 1 {
		t.Fatalf("syntax css: %v", syntax)
	}
	syn := strings.ReplaceAll(read(t, syntax[0]), " ", "")
	mustContain(t, "syntax.css", syn, "prefers-color-scheme:light", "prefers-color-scheme:dark", ".chroma")

	rss := read(t, filepath.Join(out, "writing/rss.xml"))
	mustContain(t, "rss", rss, "<title>Hello, world</title>", "<link>https://example.com/writing/hello-world/</link>", "<![CDATA[", `rel="self"`)
	mustNotContain(t, "rss", rss, "unfinished")

	sitemap := read(t, filepath.Join(out, "sitemap.xml"))
	mustContain(t, "sitemap", sitemap, "<loc>https://example.com/writing/hello-world/</loc>", "<lastmod>2026-01-01</lastmod>")
	mustNotContain(t, "sitemap", sitemap, "404", "/blog/", "unfinished")

	mustContain(t, "_redirects", read(t, filepath.Join(out, "_redirects")), "/blog/ /writing/ 301", "/blog /writing/ 301")
	mustContain(t, "redirect stub", read(t, filepath.Join(out, "blog/index.html")), `content="0; url=/writing/"`)
	mustContain(t, "robots", read(t, filepath.Join(out, "robots.txt")), "Sitemap: https://example.com/sitemap.xml")
	mustContain(t, "404", read(t, filepath.Join(out, "404.html")), `<meta name="robots" content="noindex">`)

	ico := read(t, filepath.Join(out, "favicon.ico"))
	if !strings.HasPrefix(ico, "\x00\x00\x01\x00\x03\x00") {
		t.Errorf("favicon.ico header is wrong: %q", ico[:6])
	}

	problems, err := site.Check(out, res.Site.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("check found problems: %v", problems)
	}
}

func TestDraftsOption(t *testing.T) {
	root := starter(t)
	_, out := build(t, root, site.Options{Drafts: true})
	if !exists(filepath.Join(out, "writing/unfinished/index.html")) {
		t.Fatal("draft missing with Drafts: true")
	}
}

func TestRebuildRemovesDeletedPages(t *testing.T) {
	root := starter(t)
	_, out := build(t, root, site.Options{})
	if err := os.Remove(filepath.Join(root, "content/about.md")); err != nil {
		t.Fatal(err)
	}
	build(t, root, site.Options{})
	if exists(filepath.Join(out, "about/index.html")) {
		t.Fatal("deleted page is still in the output")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".muhssg-build-*"))
	old, _ := filepath.Glob(out + ".old-*")
	if len(leftovers)+len(old) != 0 {
		t.Fatalf("temporary folders left behind: %v %v", leftovers, old)
	}
}

func TestFailedBuildKeepsPreviousOutput(t *testing.T) {
	root := starter(t)
	_, out := build(t, root, site.Options{})
	write(t, filepath.Join(root, "content/broken.md"), "---\ntitle: [oops\n---\n")
	if _, err := site.Build(site.Options{Root: root}); err == nil || !strings.Contains(err.Error(), "content/broken.md") {
		t.Fatalf("err = %v", err)
	}
	if !exists(filepath.Join(out, "index.html")) {
		t.Fatal("previous output was destroyed by a failed build")
	}
}

func TestThemeLayering(t *testing.T) {
	root := starter(t)
	// The site's own files win over the theme's.
	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}<p id="override">{{ .Title }}</p>{{ end }}`)
	write(t, filepath.Join(root, "assets/_tokens.css"), ":root { --bg: #123456; }")
	_, out := build(t, root, site.Options{})
	mustContain(t, "about", read(t, filepath.Join(out, "about/index.html")), `<p id="override">About</p>`, `<header class="site-header">`)
	css, _ := filepath.Glob(filepath.Join(out, "_assets", "site-*.css"))
	mustContain(t, "css", read(t, css[0]), "#123456")

	// A token-only theme placed first changes the look.
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	write(t, filepath.Join(root, "muhssg.yaml"), strings.Replace(cfg, "theme: [default]", "theme: [solar, default]", 1))
	os.Remove(filepath.Join(root, "assets/_tokens.css"))
	_, out = build(t, root, site.Options{})
	css, _ = filepath.Glob(filepath.Join(out, "_assets", "site-*.css"))
	mustContain(t, "solar css", read(t, css[0]), "#dc143c")
}

func TestBasePath(t *testing.T) {
	root := starter(t)
	res, out := build(t, root, site.Options{BaseURL: "https://example.github.io/blog/"})
	post := read(t, filepath.Join(out, "writing/hello-world/index.html"))
	mustContain(t, "post", post,
		`href="/blog/_assets/site-`,
		`<a class="brand" href="/blog/">`,
		`href="/blog/writing/"`,
		`<link rel="canonical" href="https://example.github.io/blog/writing/hello-world/">`,
		`href="/blog/favicon.ico"`,
	)
	mustContain(t, "_redirects", read(t, filepath.Join(out, "_redirects")), "/blog/blog/ /blog/writing/ 301")
	problems, err := site.Check(out, res.Site.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("check found problems: %v", problems)
	}
}

func TestCheckFindsBrokenLinks(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "content/about.md"), "---\ntitle: About\n---\n[gone](/nowhere/) [bad anchor](/writing/hello-world/#nope) [ok](/writing/hello-world/#code) [ext](https://example.org/x)\n")
	res, out := build(t, root, site.Options{})
	problems, err := site.Check(out, res.Site.Config)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, p := range problems {
		msgs = append(msgs, p.String())
	}
	all := strings.Join(msgs, "\n")
	mustContain(t, "problems", all, "broken link /nowhere/", "#nope")
	if len(problems) != 2 {
		t.Errorf("want 2 problems, got:\n%s", all)
	}
}

func TestPermalinksAliasesBundlesAndHTMLPages(t *testing.T) {
	root := starter(t)
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	cfg = strings.Replace(cfg, "    # permalink: /writing/:year/:slug/", "    permalink: /writing/:year/:slug/", 1)
	write(t, filepath.Join(root, "muhssg.yaml"), cfg)
	write(t, filepath.Join(root, "content/writing/trip/index.md"), "---\ntitle: Trip\ndate: 2025-06-01\naliases: [/old-trip/]\n---\n![photo](photo.png)\n")
	write(t, filepath.Join(root, "content/writing/trip/photo.png"), testPNG(t, 40, 30))
	write(t, filepath.Join(root, "content/writing/trip/notes.pdf"), "%PDF-1.4")
	write(t, filepath.Join(root, "content/lab.html"), "---\ntitle: Lab\nlayout: page\n---\n<div id=\"app\">interactive</div>\n")
	_, out := build(t, root, site.Options{})
	for _, f := range []string{"writing/2026/hello-world/index.html", "writing/2025/trip/index.html", "writing/trip/notes.pdf", "old-trip/index.html", "lab/index.html"} {
		if !exists(filepath.Join(out, f)) {
			t.Errorf("missing %s", f)
		}
	}
	mustContain(t, "alias", read(t, filepath.Join(out, "old-trip/index.html")), "url=/writing/2025/trip/")
	// The bundle image went through the image pipeline instead of being copied.
	if exists(filepath.Join(out, "writing/trip/photo.png")) {
		t.Error("processed image original was copied to the output")
	}
	mustContain(t, "trip", read(t, filepath.Join(out, "writing/2025/trip/index.html")), `width="40" height="30" loading="lazy"`)
	mustContain(t, "lab", read(t, filepath.Join(out, "lab/index.html")), `<div id="app">interactive</div>`)
}

func TestDuplicateURLsAreAnError(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "content/other.md"), "---\ntitle: Other\npermalink: /about/\n---\n")
	_, err := site.Build(site.Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), "both want the URL /about/") {
		t.Fatalf("err = %v", err)
	}
}

func TestOutputFolderGuards(t *testing.T) {
	root := starter(t)
	for _, out := range []string{root, filepath.Join(root, "content"), filepath.Dir(root)} {
		if _, err := site.Build(site.Options{Root: root, OutDir: out}); err == nil {
			t.Errorf("building into %s should fail", out)
		}
	}
}

func TestUnknownConfigKeyIsAnError(t *testing.T) {
	root := starter(t)
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	write(t, filepath.Join(root, "muhssg.yaml"), cfg+"\ntitel: typo\n")
	_, err := site.Build(site.Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), "titel") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingAssetNamesTheAsset(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}{{ css "nope.css" }}{{ end }}`)
	_, err := site.Build(site.Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), `asset "nope.css" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestCSSURLsAndRootPaths(t *testing.T) {
	root := starter(t)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	_ = png.Encode(&buf, img)
	write(t, filepath.Join(root, "assets/img/dot.png"), buf.String())
	write(t, filepath.Join(root, "assets/extra.css"), ".a{background:url(./img/dot.png)} .b{background:url(/static-file.png)}")
	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}{{ css "extra.css" }}<img src="{{ asset "img/dot.png" }}" alt="">{{ end }}`)
	_, out := build(t, root, site.Options{})
	css, _ := filepath.Glob(filepath.Join(out, "_assets", "extra-*.css"))
	if len(css) != 1 {
		t.Fatalf("extra css: %v", css)
	}
	body := read(t, css[0])
	if !regexp.MustCompile(`url\("?/_assets/files/dot-[A-Z0-9]+\.png"?\)`).MatchString(body) {
		t.Errorf("relative url not rewritten to a hashed file: %s", body)
	}
	mustContain(t, "extra.css", body, "url(/static-file.png)")
	about := read(t, filepath.Join(out, "about/index.html"))
	if !regexp.MustCompile(`src="/_assets/img/dot-[0-9a-f]{8}\.png"`).MatchString(about) {
		t.Errorf("asset() did not return a hashed URL")
	}
}

func TestBadBundleReportsFileAndLine(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "assets/app.ts"), "const x: number = ;\n")
	_, err := site.Build(site.Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), "app.ts:1:") {
		t.Fatalf("err = %v", err)
	}
}
