package site_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/apv022/muhssg/internal/site"
)

// ---- shortcodes -----------------------------------------------------------

func TestShortcodes(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "templates/shortcodes/shout.html"), `<b class="shout">{{ .Get 0 | upper }}{{ with .Get "end" }}{{ . }}{{ end }}</b>`)
	write(t, filepath.Join(root, "templates/shortcodes/box.html"), `<div class="box">{{ .InnerHTML }}</div>`)
	write(t, filepath.Join(root, "templates/shortcodes/mark.html"), `<mark>{{ .InnerInline }}</mark>`)
	write(t, filepath.Join(root, "content/sc.md"), "---\ntitle: SC\n---\n"+
		"Inline {{< shout hello end=\"!\" >}} here.\n\n"+
		"{{< youtube dQw4w9WgXcQ >}}\n\n"+
		"{{< note type=\"warning\" title=\"Careful\" >}}\nSome **bold** text.\n{{< /note >}}\n\n"+
		"{{< box >}}\nOuter {{< box >}}inner *em*{{< /box >}}\n{{< /box >}}\n\n"+
		"{{< details summary=\"More\" >}}Hidden.{{< /details >}}\n\n"+
		"A {{< mark >}}*marked*{{< /mark >}} word.\n\n"+
		"Escaped: {{</* shout raw */>}}\n\n"+
		"Inline code: `{{< shout nope >}}`\n\n"+
		"Escape in code: `{{</* shout x */>}}`\n\n"+
		"```md\n{{< shout fenced >}}\n```\n")
	_, out := build(t, root, site.Options{})
	html := read(t, filepath.Join(out, "sc/index.html"))
	mustContain(t, "sc", html,
		`Inline <b class="shout">HELLO!</b> here.`,
		`youtube-nocookie.com/embed/dQw4w9WgXcQ`,
		`<aside class="note note-warning" role="note">`,
		`<strong>Careful</strong>`,
		`Some <strong>bold</strong> text.`,
		`<div class="box"><p>Outer <div class="box"><p>inner <em>em</em></p>`,
		`<summary>More</summary>`,
		`<p>A <mark><em>marked</em></mark> word.</p>`,
		`Escaped: {{&lt; shout raw &gt;}}`,
		`<code>{{&lt; shout nope &gt;}}</code>`,
		`<code>{{&lt;/* shout x */&gt;}}</code>`,
		`{{&lt; shout fenced &gt;}}`,
	)
	mustNotContain(t, "sc", html, "MUHSSGSHORTCODE", "<p><div", `<p><aside`)
}

func TestShortcodeErrors(t *testing.T) {
	cases := map[string]string{
		"unknown":       "line 2: unknown shortcode \"nope\"",
		"unclosed tag":  "line 1: shortcode is never closed",
		"stray close":   "closes a shortcode that was never opened",
		"bad quote":     "unterminated quote",
		"template fail": "youtube: needs a video id",
	}
	bodies := map[string]string{
		"unknown":       "Hi\n{{< nope >}}\n",
		"unclosed tag":  "{{< youtube abc\n",
		"stray close":   "text {{< /note >}}\n",
		"bad quote":     "{{< youtube id=\"abc >}}\n",
		"template fail": "{{< youtube >}}\n",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			root := starter(t)
			write(t, filepath.Join(root, "content/bad.md"), "---\ntitle: Bad\n---\n"+bodies[name])
			_, err := site.Build(site.Options{Root: root})
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "content/bad.md") {
				t.Fatalf("err = %v, want it to mention content/bad.md and %q", err, want)
			}
		})
	}
}

func TestShortcodeOverrideAndHTMLContent(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "templates/shortcodes/youtube.html"), `<a class="yt" href="https://youtu.be/{{ .Get 0 }}">video</a>`)
	write(t, filepath.Join(root, "content/raw.html"), "---\ntitle: Raw\n---\n<section>{{< youtube abc >}}</section>\n")
	_, out := build(t, root, site.Options{})
	mustContain(t, "raw", read(t, filepath.Join(out, "raw/index.html")), `<section><a class="yt" href="https://youtu.be/abc">video</a></section>`)
}

// ---- images ---------------------------------------------------------------

// testJPEG makes a w×h JPEG whose left half is red and right half blue, with
// an EXIF block holding an orientation tag and a fake GPS marker.
func testJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{220, 20, 60, 255}
			if x >= w/2 {
				c = color.RGBA{20, 60, 220, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	var tiff bytes.Buffer
	tiff.WriteString("MM\x00\x2a")
	binary.Write(&tiff, binary.BigEndian, uint32(8))
	binary.Write(&tiff, binary.BigEndian, uint16(1))                                         // one IFD entry
	binary.Write(&tiff, binary.BigEndian, []uint16{0x0112, 3, 0, 1, uint16(orientation), 0}) // orientation
	binary.Write(&tiff, binary.BigEndian, uint32(0))
	tiff.WriteString("GPS-SECRET-48.8584N-2.2945E")
	app1 := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(app1)+2))
	out := append([]byte{0xFF, 0xD8}, seg...)
	out = append(out, app1...)
	return append(out, data[2:]...)
}

func TestMarkdownImagesAreResponsive(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "content/writing/photos/index.md"), "---\ntitle: Photos\ndate: 2026-03-01\n---\n![A wide photo](wide.jpg \"Title here\")\n\n![Tiny](tiny.png)\n\n![Remote](https://example.org/x.jpg)\n")
	write(t, filepath.Join(root, "content/writing/photos/wide.jpg"), string(testJPEG(t, 2000, 1000, 1)))
	write(t, filepath.Join(root, "content/writing/photos/tiny.png"), testPNG(t, 64, 48))
	_, out := build(t, root, site.Options{})
	html := read(t, filepath.Join(out, "writing/photos/index.html"))

	wide := regexp.MustCompile(`<img title="Title here" src="([^"]+)" alt="A wide photo" srcset="([^"]+)" sizes="\(min-width: 780px\) 750px, 100vw" width="(\d+)" height="(\d+)" loading="lazy" decoding="async">`).FindStringSubmatch(html)
	if wide == nil {
		t.Fatalf("wide image not made responsive:\n%s", html)
	}
	for _, w := range []string{"480w", "800w", "1200w", "1600w", "2000w"} {
		if !strings.Contains(wide[2], w) {
			t.Errorf("srcset missing %s: %s", w, wide[2])
		}
	}
	if wide[3] != "1600" || wide[4] != "800" {
		t.Errorf("src should default to the largest configured width (1600x800), got %sx%s", wide[3], wide[4])
	}
	if !strings.Contains(html, `<img src="/_assets/images/writing/photos/tiny-`) || !strings.Contains(html, `width="64" height="48"`) || strings.Contains(html, "tiny-") && strings.Contains(html, `tiny.png" srcset`) {
		t.Errorf("small image should be published once at its own size, never upscaled:\n%s", html)
	}
	mustContain(t, "remote", html, `<img src="https://example.org/x.jpg" alt="Remote">`)

	// Every published variant is re-encoded: no EXIF, so no GPS data.
	files, _ := filepath.Glob(filepath.Join(out, "_assets/images/writing/photos/wide-*.jpg"))
	if len(files) != 5 {
		t.Fatalf("want 5 wide variants, got %v", files)
	}
	for _, f := range files {
		if bytes.Contains([]byte(read(t, f)), []byte("GPS-SECRET")) {
			t.Errorf("%s still contains EXIF data", filepath.Base(f))
		}
	}
	if exists(filepath.Join(out, "writing/photos/wide.jpg")) {
		t.Error("the original (with GPS data) was copied to the output")
	}
}

func TestEXIFOrientation(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "assets/img/phone.jpg"), string(testJPEG(t, 400, 200, 6))) // rotate 90° clockwise
	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}{{ with image "img/phone.jpg" }}<p id="size">{{ .Width }}x{{ .Height }} {{ .URL }}</p>{{ with .Resize 100 }}<p id="small">{{ .Width }}x{{ .Height }}</p>{{ end }}{{ end }}{{ end }}`)
	_, out := build(t, root, site.Options{})
	html := read(t, filepath.Join(out, "about/index.html"))
	mustContain(t, "about", html, `<p id="size">200x400 /_assets/images/img/phone-`, `<p id="small">100x200</p>`)

	// After rotating 90° clockwise, the red left half is now the top half.
	m := regexp.MustCompile(`/_assets/images/(img/phone-[0-9a-f]+-200\.jpg)`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("full-size URL not found")
	}
	f, err := os.Open(filepath.Join(out, "_assets/images", filepath.FromSlash(m[1])))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := img.At(100, 20).RGBA()
	if r>>8 < 150 || b>>8 > 120 {
		t.Errorf("top of rotated image should be red, got r=%d b=%d", r>>8, b>>8)
	}
}

func TestImageFunctionsAndErrors(t *testing.T) {
	root := starter(t)
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	cfg = regexp.MustCompile(`(?s)images:\n.*?\n\n`).ReplaceAllString(cfg, "images:\n  widths: [100, 300]\n  quality: 70\n  sizes: \"(min-width: 700px) 640px, 100vw\"\n\n")
	if !strings.Contains(cfg, "widths: [100, 300]") {
		t.Fatal("starter images block not found")
	}
	write(t, filepath.Join(root, "muhssg.yaml"), cfg)
	write(t, filepath.Join(root, "assets/img/pic.png"), testPNG(t, 500, 250))
	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}{{ img "img/pic.png" "A \"pic\"" }}{{ end }}`)
	_, out := build(t, root, site.Options{})
	mustContain(t, "about", read(t, filepath.Join(out, "about/index.html")),
		`alt="A &#34;pic&#34;"`, ` 100w, `, ` 300w, `, ` 500w"`, `sizes="(min-width: 700px) 640px, 100vw"`, `width="300" height="150"`)

	write(t, filepath.Join(root, "templates/page.html"), `{{ define "main" }}{{ img "img/missing.png" "x" }}{{ end }}`)
	if _, err := site.Build(site.Options{Root: root}); err == nil || !strings.Contains(err.Error(), `image "img/missing.png" not found`) {
		t.Fatalf("err = %v", err)
	}
}

// ---- pagination -----------------------------------------------------------

func TestPagination(t *testing.T) {
	root := starter(t)
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	cfg = strings.Replace(cfg, "    paginate: 10", "    paginate: 2", 1)
	write(t, filepath.Join(root, "muhssg.yaml"), cfg)
	for i := 1; i <= 4; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("content/writing/post-%d.md", i)), fmt.Sprintf("---\ntitle: Post %d\ndate: 2026-02-0%d\ntags: [many]\n---\nBody.\n", i, i))
	}
	write(t, filepath.Join(root, "templates/list.html"), `{{ define "main" }}<p id="pg">{{ .Paginator.PageNumber }}/{{ .Paginator.TotalPages }} of {{ .Paginator.TotalItems }}</p><ul>{{ range .Paginator.Items }}<li>{{ .Title }}</li>{{ end }}</ul>{{ with .Paginator.PrevURL }}<a rel="prev" href="{{ . }}">prev</a>{{ end }}{{ with .Paginator.NextURL }}<a rel="next" href="{{ . }}">next</a>{{ end }}<nav>{{ range .Paginator.Links }}<a href="{{ .URL }}"{{ if .Current }} aria-current="page"{{ end }}>{{ .Number }}</a>{{ end }}</nav>{{ end }}`)
	res, out := build(t, root, site.Options{})

	p1 := read(t, filepath.Join(out, "writing/index.html"))
	p2 := read(t, filepath.Join(out, "writing/page/2/index.html"))
	p3 := read(t, filepath.Join(out, "writing/page/3/index.html"))
	mustContain(t, "page 1", p1, `<p id="pg">1/3 of 5</p>`, "<li>Post 4</li><li>Post 3</li>", `href="/writing/page/2/">next`, `<a href="/writing/" aria-current="page">1</a>`)
	mustNotContain(t, "page 1", p1, `rel="prev"`)
	mustContain(t, "page 2", p2, `<p id="pg">2/3 of 5</p>`, "<li>Post 2</li><li>Post 1</li>", `href="/writing/">prev`, `href="/writing/page/3/">next`)
	mustContain(t, "page 3", p3, `<p id="pg">3/3 of 5</p>`, "<li>Hello, world</li>")
	mustNotContain(t, "page 3", p3, `rel="next"`)
	mustContain(t, "page/1 alias", read(t, filepath.Join(out, "writing/page/1/index.html")), "url=/writing/")
	mustContain(t, "sitemap", read(t, filepath.Join(out, "sitemap.xml")), "/writing/page/2/")
	if exists(filepath.Join(out, "writing/page/4/index.html")) {
		t.Error("too many pages")
	}

	// Tag pages paginate with the site-wide setting.
	write(t, filepath.Join(root, "muhssg.yaml"), cfg+"\npaginate: 3\n")
	_, out = build(t, root, site.Options{})
	if !exists(filepath.Join(out, "tags/many/page/2/index.html")) {
		t.Error("tag page was not paginated")
	}
	problems, err := site.Check(out, res.Site.Config)
	if err != nil || len(problems) != 0 {
		t.Errorf("check: %v %v", err, problems)
	}
}

func TestPaginationUnderBasePath(t *testing.T) {
	root := starter(t)
	cfg := read(t, filepath.Join(root, "muhssg.yaml"))
	write(t, filepath.Join(root, "muhssg.yaml"), strings.Replace(cfg, "    paginate: 10", "    paginate: 1", 1))
	write(t, filepath.Join(root, "content/writing/second.md"), "---\ntitle: Second\ndate: 2026-05-01\n---\nx\n")
	write(t, filepath.Join(root, "templates/list.html"), `{{ define "main" }}{{ with .Paginator.NextURL }}<a id="next" href="{{ . }}">next</a>{{ end }}{{ end }}`)
	res, out := build(t, root, site.Options{BaseURL: "https://example.github.io/blog/"})
	mustContain(t, "list", read(t, filepath.Join(out, "writing/index.html")), `<a id="next" href="/blog/writing/page/2/">`)
	problems, _ := site.Check(out, res.Site.Config)
	if len(problems) != 0 {
		t.Errorf("check: %v", problems)
	}
}

// Under a base path, links written in Markdown get the prefix once, and
// shortcode output (which uses relURL) and processed images are not
// prefixed a second time.
func TestBasePathWithShortcodesAndImages(t *testing.T) {
	root := starter(t)
	write(t, filepath.Join(root, "templates/shortcodes/go.html"), `<a class="go" href="{{ relURL (.Get "url") }}">{{ .InnerInline }}</a>`)
	write(t, filepath.Join(root, "static/img/root.png"), testPNG(t, 40, 20))
	write(t, filepath.Join(root, "content/writing/pics/index.md"), "---\ntitle: Pics\ndate: 2026-06-01\n---\n"+
		"[home](/) and [about](/about/)\n\n"+
		"{{< go url=\"/about/\" >}}About{{< /go >}}\n\n"+
		"{{< note >}}Inner [link](/writing/).{{< /note >}}\n\n"+
		"![bundle](bundle.png)\n\n![root](/img/root.png)\n")
	write(t, filepath.Join(root, "content/writing/pics/bundle.png"), testPNG(t, 30, 30))
	write(t, filepath.Join(root, "content/raw.html"), "---\ntitle: Raw\n---\n<a id=\"r\" href=\"/about/\">x</a> {{< go url=\"/writing/\" >}}W{{< /go >}}\n")
	res, out := build(t, root, site.Options{BaseURL: "https://example.github.io/blog/"})
	page := read(t, filepath.Join(out, "writing/pics/index.html"))
	mustContain(t, "pics", page,
		`<a href="/blog/">home</a>`, `<a href="/blog/about/">about</a>`,
		`<a class="go" href="/blog/about/">About</a>`,
		`<a href="/blog/writing/">link</a>`,
		`src="/blog/_assets/images/writing/pics/bundle-`,
		`src="/blog/_assets/images/img/root-`,
	)
	mustNotContain(t, "pics", page, "/blog/blog/")
	raw := read(t, filepath.Join(out, "raw/index.html"))
	mustContain(t, "raw", raw, `<a id="r" href="/blog/about/">x</a>`, `<a class="go" href="/blog/writing/">W</a>`)
	mustNotContain(t, "raw", raw, "/blog/blog/")
	problems, err := site.Check(out, res.Site.Config)
	if err != nil || len(problems) != 0 {
		t.Errorf("check: %v %v", err, problems)
	}
}
