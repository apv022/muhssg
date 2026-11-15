package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"
	"time"
)

//go:embed builtin/templates
var builtinFS embed.FS

// templateSource is one template file after layering.
type templateSource struct {
	name string // slash path relative to templates/, e.g. "writing/single.html"
	file string // absolute path, for error messages
	text string
}

// loadTemplateSources reads templates/ from every layer; earlier layers
// (the site) override later ones (themes).
func loadTemplateSources(cfg *Config) (map[string]templateSource, error) {
	out := map[string]templateSource{}
	// Built-in shortcodes are the lowest layer: any theme or site can override them.
	err := fs.WalkDir(builtinFS, "builtin/templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := builtinFS.ReadFile(p)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(p, "builtin/templates/")
		out[name] = templateSource{name: name, file: "(built-in) templates/" + name, text: string(raw)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	layers := cfg.LayerDirs()
	for i := len(layers) - 1; i >= 0; i-- {
		dir := filepath.Join(layers[i], "templates")
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
				return err
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, p)
			name := filepath.ToSlash(rel)
			out[name] = templateSource{name: name, file: p, text: string(raw)}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func isShared(name string) bool {
	return name == "base.html" || strings.HasPrefix(name, "partials/")
}

func isShortcode(name string) bool { return strings.HasPrefix(name, "shortcodes/") }

// templates holds one parsed set per layout. Each set contains base.html,
// every partial, and the layout itself.
type templates struct {
	sets       map[string]*template.Template
	shortcodes map[string]*template.Template
	entry      map[string]string // layout -> template to execute
	sources    map[string]templateSource
}

func parseTemplates(sources map[string]templateSource, funcs template.FuncMap) (*templates, error) {
	shared := template.New("").Funcs(funcs)
	names := sortedKeys(sources)
	for _, name := range names {
		if !isShared(name) {
			continue
		}
		src := sources[name]
		if _, err := shared.New(name).Parse(src.text); err != nil {
			return nil, templateError(src, err)
		}
	}
	t := &templates{sets: map[string]*template.Template{}, shortcodes: map[string]*template.Template{}, entry: map[string]string{}, sources: sources}
	for _, name := range names {
		if isShared(name) {
			continue
		}
		if isShortcode(name) {
			src := sources[name]
			set, err := shared.Clone()
			if err != nil {
				return nil, err
			}
			if _, err := set.New(name).Parse(src.text); err != nil {
				return nil, templateError(src, err)
			}
			t.shortcodes[name] = set
			continue
		}
		src := sources[name]
		set, err := shared.Clone()
		if err != nil {
			return nil, err
		}
		layout, err := set.New(name).Parse(src.text)
		if err != nil {
			return nil, templateError(src, err)
		}
		t.sets[name] = set
		if onlyDefines(layout) {
			if set.Lookup("base.html") == nil {
				return nil, fmt.Errorf("%s only defines blocks, but there is no base.html to put them in", src.file)
			}
			t.entry[name] = "base.html"
		} else {
			t.entry[name] = name
		}
	}
	return t, nil
}

// onlyDefines reports whether a template's body is empty apart from
// {{define}} blocks and whitespace, meaning it fills in base.html.
func onlyDefines(t *template.Template) bool {
	if t.Tree == nil || t.Tree.Root == nil {
		return true
	}
	for _, n := range t.Tree.Root.Nodes {
		if tn, ok := n.(*parse.TextNode); ok && len(bytes.TrimSpace(tn.Text)) == 0 {
			continue
		}
		return false
	}
	return true
}

func templateError(src templateSource, err error) error {
	return fmt.Errorf("%s: %w", src.file, err)
}

func (t *templates) has(name string) bool {
	_, ok := t.sources[name]
	return ok && !isShared(name) && !isShortcode(name)
}

// candidates lists the layouts tried for a page, most specific first.
func candidates(p *Page) []string {
	var c []string
	if p.Layout != "" {
		l := p.Layout
		if !strings.HasSuffix(l, ".html") {
			l += ".html"
		}
		if p.Section != "" {
			c = append(c, p.Section+"/"+l)
		}
		return append(c, l)
	}
	switch p.Kind {
	case KindHome:
		c = []string{"index.html", "home.html", "page.html"}
	case KindPage:
		c = []string{"page.html"}
	case KindSingle:
		c = []string{p.Section + "/single.html", "single.html", "page.html"}
	case KindList:
		c = []string{p.Section + "/list.html", "list.html"}
	case KindNotFound:
		c = []string{"404.html", "page.html"}
	case KindTag:
		c = []string{"tag.html"}
	case KindTags:
		c = []string{"tags.html"}
	}
	return c
}

func (t *templates) render(p *Page) ([]byte, error) {
	cands := candidates(p)
	for _, name := range cands {
		set, ok := t.sets[name]
		if !ok {
			continue
		}
		var buf bytes.Buffer
		if err := set.ExecuteTemplate(&buf, t.entry[name], p); err != nil {
			return nil, fmt.Errorf("%s: rendering with templates/%s: %w", describe(p), name, err)
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("%s: no template found (looked for %s in templates/)", describe(p), strings.Join(cands, ", "))
}

// YearGroup is returned by groupByYear.
type YearGroup struct {
	Year  int
	Pages []*Page
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

type renderEnv struct {
	site     *Site
	manifest Manifest
	favicons faviconSet
	content  *contentRenderer
	images   *imageProc
}

// splitPageArg lets image functions take an optional page first:
// img "photo.jpg" "alt" or img .Page "photo.jpg" "alt".
func splitPageArg(name string, args []any) (*Page, []string, error) {
	var p *Page
	if len(args) > 0 {
		if pg, ok := args[0].(*Page); ok {
			p, args = pg, args[1:]
		}
	}
	out := make([]string, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			out[i] = v
		case template.HTML:
			out[i] = string(v)
		case nil:
		default:
			return nil, nil, fmt.Errorf("%s: argument %d is a %T, not a string", name, i+1, a)
		}
	}
	if len(out) == 0 || out[0] == "" {
		return nil, nil, fmt.Errorf("%s: needs an image path", name)
	}
	return p, out, nil
}

func (e *renderEnv) asset(name string) (string, error) {
	name = strings.TrimPrefix(name, "/")
	if u, ok := e.manifest[name]; ok {
		return u, nil
	}
	var known []string
	for k := range e.manifest {
		known = append(known, k)
	}
	sort.Strings(known)
	if len(known) == 0 {
		return "", fmt.Errorf("asset %q not found: there is no assets/ folder", name)
	}
	return "", fmt.Errorf("asset %q not found in assets/ (have: %s)", name, strings.Join(known, ", "))
}

func (e *renderEnv) funcs() template.FuncMap {
	s := e.site
	return template.FuncMap{
		// assets
		"asset": e.asset,
		"hasAsset": func(name string) bool {
			_, ok := e.manifest[strings.TrimPrefix(name, "/")]
			return ok
		},
		"css": func(name string) (template.HTML, error) {
			u, err := e.asset(name)
			if err != nil {
				return "", err
			}
			return template.HTML(`<link rel="stylesheet" href="` + template.HTMLEscapeString(u) + `">`), nil
		},
		"js": func(name string) (template.HTML, error) {
			u, err := e.asset(name)
			if err != nil {
				return "", err
			}
			return template.HTML(`<script type="module" src="` + template.HTMLEscapeString(u) + `"></script>`), nil
		},
		"favicons": func() template.HTML { return e.favicons.html(s.Config.BasePath) },
		"seo":      func(p *Page) template.HTML { return seoTags(p) },
		"fullTitle": func(p *Page) string {
			if p.Kind == KindHome || p.Title == "" || p.Title == s.Title || s.Title == "" {
				return firstNonEmpty(p.Title, s.Title)
			}
			return p.Title + " — " + s.Title
		},

		// URLs
		"absURL": s.AbsURL,
		"relURL": s.RelURL,
		"active": func(p *Page, prefix string) bool {
			prefix = cleanURLPath(prefix)
			if prefix == "/" {
				return p.Path == "/"
			}
			return strings.HasPrefix(p.Path, prefix)
		},
		"tagURL": func(name string) string { return s.RelURL("/tags/" + Slugify(name) + "/") },
		"urlize": Slugify,

		// pages
		"collection": func(name string) []*Page {
			if c := s.Collections[name]; c != nil {
				return c.Pages
			}
			return nil
		},
		"page":  s.GetPage,
		"first": func(n int, pages []*Page) []*Page { return pages[:min(n, len(pages))] },
		"after": func(n int, pages []*Page) []*Page { return pages[min(n, len(pages)):] },
		"reverse": func(pages []*Page) []*Page {
			out := make([]*Page, len(pages))
			for i, p := range pages {
				out[len(pages)-1-i] = p
			}
			return out
		},
		"sortBy": func(field string, pages []*Page) ([]*Page, error) {
			out := append([]*Page(nil), pages...)
			order := "asc"
			if strings.HasPrefix(field, "-") {
				order, field = "desc", field[1:]
			}
			switch strings.ToLower(field) {
			case "date", "title", "weight":
				sortPages(out, strings.ToLower(field), order)
			default:
				return nil, fmt.Errorf("sortBy: %q is not Date, Title or Weight", field)
			}
			return out, nil
		},
		"where": func(field, value string, pages []*Page) ([]*Page, error) {
			var out []*Page
			for _, p := range pages {
				var match bool
				switch strings.ToLower(field) {
				case "section":
					match = p.Section == value
				case "kind":
					match = p.Kind == value
				case "tag":
					for _, t := range p.Tags {
						if Slugify(t) == Slugify(value) {
							match = true
						}
					}
				default:
					return nil, fmt.Errorf("where: %q is not Section, Kind or Tag", field)
				}
				if match {
					out = append(out, p)
				}
			}
			return out, nil
		},
		"groupByYear": func(pages []*Page) []YearGroup {
			var out []YearGroup
			for _, p := range pages {
				y := p.Date.Year()
				if len(out) == 0 || out[len(out)-1].Year != y {
					out = append(out, YearGroup{Year: y})
				}
				out[len(out)-1].Pages = append(out[len(out)-1].Pages, p)
			}
			return out
		},

		// dates
		"date": func(layout string, t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format(layout)
		},
		"isoDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"now": time.Now,

		// text
		"markdown": func(src string) (template.HTML, error) {
			out, err := e.content.renderFragment(nil, src, 1)
			return template.HTML(out), err
		},
		"markdownInline": func(src string) (template.HTML, error) {
			out, err := e.content.renderFragment(nil, src, 1)
			return template.HTML(stripParagraph(out)), err
		},
		"errorf": func(format string, args ...any) (string, error) {
			return "", fmt.Errorf(format, args...)
		},

		// images
		"image": func(args ...any) (*Image, error) {
			p, a, err := splitPageArg("image", args)
			if err != nil {
				return nil, err
			}
			return e.images.original(p, a[0])
		},
		"img": func(args ...any) (template.HTML, error) {
			p, a, err := splitPageArg("img", args)
			if err != nil {
				return "", err
			}
			for len(a) < 4 {
				a = append(a, "")
			}
			return e.images.imgTag(p, a[0], a[1], a[2], a[3])
		},
		"plainify": func(s string) string { return collapseSpace(tagRe.ReplaceAllString(s, " ")) },
		"truncate": func(n int, s string) string { return Truncate(s, n) },
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"trim":     strings.TrimSpace,
		"replace":  func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"split":    func(sep, s string) []string { return strings.Split(s, sep) },
		"join": func(sep string, v any) (string, error) {
			switch list := v.(type) {
			case []string:
				return strings.Join(list, sep), nil
			case []any:
				parts := make([]string, len(list))
				for i, x := range list {
					parts[i] = fmt.Sprint(x)
				}
				return strings.Join(parts, sep), nil
			}
			return "", fmt.Errorf("join: cannot join %T", v)
		},
		// pattern first, so they chain: {{ if .Title | contains "Go" }}
		"contains":  func(sub, s string) bool { return strings.Contains(s, sub) },
		"hasPrefix": func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"hasSuffix": func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },

		// values
		"default": func(def, v any) any {
			if isEmpty(v) {
				return def
			}
			return v
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict: needs key/value pairs")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"list": func(v ...any) []any { return v },
		"jsonify": func(v any) (template.JS, error) {
			b, err := json.Marshal(v)
			return template.JS(b), err
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },

		// trust (use only with content you wrote)
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"safeURL":  func(s string) template.URL { return template.URL(s) },
		"safeCSS":  func(s string) template.CSS { return template.CSS(s) },
		"safeJS":   func(s string) template.JS { return template.JS(s) },
	}
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case template.HTML:
		return strings.TrimSpace(string(x)) == ""
	case bool:
		return !x
	case int:
		return x == 0
	case float64:
		return x == 0
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case []*Page:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case time.Time:
		return x.IsZero()
	}
	return false
}

// seoTags renders description, canonical, Open Graph, Twitter and feed tags.
func seoTags(p *Page) template.HTML {
	s := p.Site
	esc := template.HTMLEscapeString
	var b strings.Builder
	meta := func(attr, key, val string) {
		if strings.TrimSpace(val) != "" {
			fmt.Fprintf(&b, `<meta %s="%s" content="%s">`+"\n", attr, key, esc(val))
		}
	}
	desc := firstNonEmpty(p.Description, Truncate(p.Summary, 160), s.Description)
	title := firstNonEmpty(p.Title, s.Title)
	meta("name", "description", desc)
	if s.BaseURL != "" && p.Kind != KindNotFound {
		fmt.Fprintf(&b, `<link rel="canonical" href="%s">`+"\n", esc(p.Permalink()))
	}
	if p.NoIndex {
		meta("name", "robots", "noindex")
	}
	ogType := "website"
	if p.Kind == KindSingle {
		ogType = "article"
	}
	meta("property", "og:type", ogType)
	meta("property", "og:site_name", s.Title)
	meta("property", "og:title", title)
	meta("property", "og:description", desc)
	if s.BaseURL != "" {
		meta("property", "og:url", p.Permalink())
	}
	if s.Language != "" {
		meta("property", "og:locale", strings.ReplaceAll(s.Language, "-", "_"))
	}
	image := firstNonEmpty(p.Image, s.Image)
	if image != "" {
		meta("property", "og:image", s.AbsURL(image))
	}
	if p.Kind == KindSingle && !p.Date.IsZero() {
		meta("property", "article:published_time", p.Date.Format(time.RFC3339))
		if !p.Updated.IsZero() {
			meta("property", "article:modified_time", p.Updated.Format(time.RFC3339))
		}
	}
	card := "summary"
	if image != "" {
		card = "summary_large_image"
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:title", title)
	meta("name", "twitter:description", desc)
	if image != "" {
		meta("name", "twitter:image", s.AbsURL(image))
	}
	for _, f := range s.feeds {
		ft := firstNonEmpty(f.Title, s.Title)
		fmt.Fprintf(&b, `<link rel="alternate" type="application/rss+xml" title="%s" href="%s">`+"\n", esc(ft), esc(s.RelURL(f.Path)))
	}
	meta("name", "generator", s.Generator)
	return template.HTML(b.String())
}
