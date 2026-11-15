package site

import (
	"strings"
	"testing"
	"time"
)

func TestSplitFrontMatter(t *testing.T) {
	cases := []struct {
		name, src, fm, body string
		has, wantErr        bool
	}{
		{"lf", "---\ntitle: A\n---\nbody\n", "title: A\n", "body\n", true, false},
		{"crlf", "---\r\ntitle: A\r\n---\r\nbody\r\n", "title: A\n", "body\n", true, false},
		{"bom", "\xef\xbb\xbf---\ntitle: A\n---\nbody", "title: A\n", "body", true, false},
		{"closing at eof", "---\ntitle: A\n---", "title: A\n", "", true, false},
		{"trailing spaces on delimiter", "---\ntitle: A\n---  \nbody", "title: A\n", "body", true, false},
		{"empty front matter", "---\n---\nbody", "", "body", true, false},
		{"none", "# Hello\n", "", "# Hello\n", false, false},
		{"dashes later are not front matter", "intro\n---\nmore", "", "intro\n---\nmore", false, false},
		{"unclosed", "---\ntitle: A\nbody", "", "", true, true},
		{"only opener", "---", "", "", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm, body, has, err := SplitFrontMatter([]byte(c.src))
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if has != c.has || string(fm) != c.fm || string(body) != c.body {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", fm, body, has, c.fm, c.body, c.has)
			}
		})
	}
}

func TestParseFrontMatter(t *testing.T) {
	meta, params, err := ParseFrontMatter([]byte("title: Hi\ndate: 2026-08-29\nupdated: \"2026-09-01 10:30\"\ntags: [a, b]\nhero: big\n"))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Hi" || len(meta.Tags) != 2 {
		t.Fatalf("meta = %+v", meta)
	}
	if !meta.Date.Equal(time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %v", meta.Date)
	}
	if !meta.Updated.Equal(time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("updated = %v", meta.Updated)
	}
	if params["hero"] != "big" || params["title"] != nil {
		t.Fatalf("params = %v", params)
	}
}

func TestParseFrontMatterBadDate(t *testing.T) {
	_, _, err := ParseFrontMatter([]byte("date: last tuesday\n"))
	if err == nil || !strings.Contains(err.Error(), "not a date") {
		t.Fatalf("err = %v", err)
	}
}

func TestCleanURLPath(t *testing.T) {
	for in, want := range map[string]string{
		"":          "/",
		"/":         "/",
		"about":     "/about/",
		"/about":    "/about/",
		"/a//b/":    "/a/b/",
		"/feed.xml": "/feed.xml",
		"/404.html": "/404.html",
		"/x/../y":   "/y/",
		"/notes.d/": "/notes.d/",
	} {
		if got := cleanURLPath(in); got != want {
			t.Errorf("cleanURLPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Hello, World!": "hello-world",
		"  Go 1.24  ":   "go-1-24",
		"Ünïcode café":  "ünïcode-café",
		"---":           "",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Fatal(got)
	}
	got := Truncate("the quick brown fox jumps over the lazy dog", 20)
	if got != "the quick brown fox…" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderMarkdown(t *testing.T) {
	cfg := &Config{}
	md := newMarkdown(cfg)
	r, err := RenderMarkdown(md, []byte("First *para* here.\n\n## A heading\n\n```go\nfunc main() {}\n```\n\nMore words.[^1]\n\n[^1]: Note.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != "First para here." {
		t.Errorf("summary = %q", r.Summary)
	}
	if len(r.Headings) != 1 || r.Headings[0].ID != "a-heading" || r.Headings[0].Level != 2 {
		t.Errorf("headings = %+v", r.Headings)
	}
	for _, want := range []string{`<h2 id="a-heading">`, `class="chroma"`, `footnote`} {
		if !strings.Contains(r.HTML, want) {
			t.Errorf("html missing %q:\n%s", want, r.HTML)
		}
	}
	more, _ := RenderMarkdown(md, []byte("One.\n\nTwo.\n<!--more-->\nThree."))
	if more.Summary != "One. Two." {
		t.Errorf("more summary = %q", more.Summary)
	}
}

func TestEncodeICO(t *testing.T) {
	ico := encodeICO([]int{16, 32}, [][]byte{[]byte("aaaa"), []byte("bbbbbb")})
	if len(ico) != 6+2*16+10 || ico[2] != 1 || ico[4] != 2 {
		t.Fatalf("bad header: % x", ico[:6])
	}
	if ico[6] != 16 || ico[22] != 32 {
		t.Fatalf("bad sizes")
	}
}
