package site

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

// Site is available in templates as .Site.
type Site struct {
	Title       string
	Description string
	Language    string
	Author      string
	BaseURL     string
	Image       string
	Params      map[string]any
	Data        map[string]any
	Pages       []*Page // every page that gets rendered
	Collections map[string]*Collection
	Tags        []*Tag
	Home        *Page
	BuildTime   time.Time
	Dev         bool // true under `muhssg serve`
	Generator   string

	Config  *Config
	aliases map[string]string // redirect from -> page path
	feeds   []feedInfo
}

type feedInfo struct {
	Title string
	Path  string
	Coll  *Collection
}

// AbsURL turns a site path like "/writing/" into an absolute URL. Without a
// baseURL it returns the path with the base path prepended.
func (s *Site) AbsURL(p string) string {
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return s.Config.Origin + s.Config.BasePath + p
}

// RelURL prepends the base path to a site path like "/writing/".
func (s *Site) RelURL(p string) string {
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") || !strings.HasPrefix(p, "/") {
		return p
	}
	return s.Config.BasePath + p
}

// Collection returns a collection by name, or nil.
func (s *Site) Collection(name string) *Collection { return s.Collections[name] }

// GetPage finds a page by its site path ("/about/") or content file ("about.md").
func (s *Site) GetPage(ref string) *Page {
	for _, p := range s.Pages {
		if p.Path == ref || p.Source == strings.TrimPrefix(ref, "/") || p.Path == cleanURLPath(ref) {
			return p
		}
	}
	return nil
}

// RegularPages returns standalone pages and collection entries (no lists,
// tags, home or 404), newest first.
func (s *Site) RegularPages() []*Page {
	var out []*Page
	for _, p := range s.Pages {
		if p.Kind == KindPage || p.Kind == KindSingle {
			out = append(out, p)
		}
	}
	sortPages(out, "date", "desc")
	return out
}

type modelOptions struct {
	drafts      bool
	hasTemplate func(name string) bool
}

func buildModel(cfg *Config, raws []rawPage, opts modelOptions) (*Site, error) {
	s := &Site{
		Title:       cfg.Title,
		Description: cfg.Description,
		Language:    cfg.Language,
		Author:      cfg.Author,
		BaseURL:     cfg.BaseURL,
		Image:       cfg.Image,
		Params:      cfg.Params,
		Data:        map[string]any{},
		Collections: map[string]*Collection{},
		BuildTime:   time.Now(),
		Config:      cfg,
		aliases:     map[string]string{},
	}
	md := newMarkdown(cfg)
	listMeta := map[string]*rawPage{}

	for i := range raws {
		r := &raws[i]
		segs := strings.Split(r.rel, "/")
		name := strings.TrimSuffix(segs[len(segs)-1], path.Ext(r.rel))

		// _index.md, or index.md directly inside a collection folder, holds list metadata.
		if len(segs) == 2 && (r.isIndex || name == "index") {
			listMeta[segs[0]] = r
			continue
		}
		if r.isIndex {
			if len(segs) == 1 {
				return nil, fmt.Errorf("content/_index.md: use content/index.md for the home page")
			}
			return nil, fmt.Errorf("content/%s: _index.md only works directly inside a collection folder", r.rel)
		}
		if r.meta.Draft && !opts.drafts {
			continue
		}

		p := &Page{
			Title:       r.meta.Title,
			Description: r.meta.Description,
			Date:        r.meta.Date.Time,
			Updated:     r.meta.Updated.Time,
			Draft:       r.meta.Draft,
			Tags:        r.meta.Tags,
			Aliases:     r.meta.Aliases,
			Weight:      r.meta.Weight,
			NoIndex:     r.meta.NoIndex,
			ShowTOC:     r.meta.TOC,
			Layout:      r.meta.Layout,
			Image:       r.meta.Image,
			Params:      r.params,
			Source:      r.rel,
			Site:        s,
			body:        r.body,
			isHTML:      r.isHTML,
		}

		filePath := ""
		switch {
		case len(segs) == 1 && name == "index":
			p.Kind, p.Path = KindHome, "/"
		case len(segs) == 1 && name == "404":
			p.Kind, p.Path, p.NoIndex = KindNotFound, "/404.html", true
		case len(segs) == 1:
			p.Kind = KindPage
			p.Slug = name
			p.Path = cleanURLPath("/" + name + "/")
		default:
			p.Kind = KindSingle
			p.Section = segs[0]
			dirs := segs[1 : len(segs)-1]
			slug := name
			if name == "index" { // page bundle: content/writing/my-post/index.md
				slug = dirs[len(dirs)-1]
				dirs = dirs[:len(dirs)-1]
			}
			if r.meta.Slug != "" {
				slug = r.meta.Slug
			}
			p.Slug = slug
			filePath = strings.Join(append(append([]string{}, dirs...), slug), "/")
			pattern := cfg.Collection(p.Section).Permalink
			if pattern == "" {
				pattern = "/:section/:filepath/"
			}
			pp, err := expandPermalink(pattern, p, filePath)
			if err != nil {
				return nil, fmt.Errorf("content/%s: %w", r.rel, err)
			}
			p.Path = pp
		}
		if p.Kind == KindPage && r.meta.Slug != "" {
			p.Slug = r.meta.Slug
			p.Path = cleanURLPath("/" + r.meta.Slug + "/")
		}
		if r.meta.Permalink != "" {
			p.Path = cleanURLPath(r.meta.Permalink)
		}
		if p.Title == "" {
			switch p.Kind {
			case KindHome:
				p.Title = cfg.Title
			case KindNotFound:
				p.Title = "Not found"
			default:
				p.Title = titleFromSlug(p.Slug)
			}
		}
		if err := renderBody(md, p, r.meta.Summary); err != nil {
			return nil, fmt.Errorf("content/%s: %w", r.rel, err)
		}
		s.Pages = append(s.Pages, p)
		if p.Kind == KindHome {
			s.Home = p
		}
		if p.Section != "" {
			c := s.collection(p.Section)
			c.Pages = append(c.Pages, p)
		}
	}

	// Collections: folders with entries, _index.md files, and config entries.
	for name, r := range listMeta {
		c := s.collection(name)
		if r.meta.Title != "" {
			c.Title = r.meta.Title
		}
		if r.meta.Description != "" {
			c.Description = r.meta.Description
		}
	}
	for name := range cfg.Collections {
		s.collection(name)
	}
	for _, name := range sortedKeys(s.Collections) {
		c := s.Collections[name]
		sortPages(c.Pages, c.Config.Sort, c.Config.Order)
		for i, p := range c.Pages {
			if i > 0 {
				p.Prev = c.Pages[i-1]
			}
			if i < len(c.Pages)-1 {
				p.Next = c.Pages[i+1]
			}
		}
		r := listMeta[name]
		_, configured := cfg.Collections[name]
		if r == nil && !configured && !opts.hasTemplate(name+"/list.html") {
			continue
		}
		lp := &Page{
			Kind:        KindList,
			Title:       c.Title,
			Description: c.Description,
			Section:     name,
			Slug:        name,
			Path:        "/" + name + "/",
			Pages:       c.Pages,
			Params:      map[string]any{},
			Site:        s,
		}
		if r != nil {
			lp.Source = r.rel
			lp.Params = r.params
			lp.Layout = r.meta.Layout
			lp.Image = r.meta.Image
			lp.NoIndex = r.meta.NoIndex
			lp.Aliases = r.meta.Aliases
			lp.Date = r.meta.Date.Time
			if r.meta.Permalink != "" {
				lp.Path = cleanURLPath(r.meta.Permalink)
			}
			lp.body, lp.isHTML = r.body, r.isHTML
			if err := renderBody(md, lp, r.meta.Summary); err != nil {
				return nil, fmt.Errorf("content/%s: %w", r.rel, err)
			}
		}
		if len(c.Pages) > 0 && lp.Date.IsZero() {
			lp.Date = latest(c.Pages)
		}
		c.List = lp
		s.Pages = append(s.Pages, lp)
		if c.Config.Feed {
			s.feeds = append(s.feeds, feedInfo{Title: c.Title, Path: cleanURLPath(c.Config.FeedPath), Coll: c})
		}
	}

	// Tags.
	tagMap := map[string]*Tag{}
	for _, p := range s.Pages {
		if p.Kind != KindSingle && p.Kind != KindPage {
			continue
		}
		for _, name := range p.Tags {
			slug := Slugify(name)
			if slug == "" {
				continue
			}
			t := tagMap[slug]
			if t == nil {
				t = &Tag{Name: name, Slug: slug, URL: s.RelURL("/tags/" + slug + "/")}
				tagMap[slug] = t
			}
			t.Pages = append(t.Pages, p)
		}
	}
	for _, slug := range sortedKeys(tagMap) {
		t := tagMap[slug]
		sortPages(t.Pages, "date", "desc")
		s.Tags = append(s.Tags, t)
		if opts.hasTemplate("tag.html") {
			s.Pages = append(s.Pages, &Page{
				Kind: KindTag, Title: t.Name, Slug: t.Slug, Path: "/tags/" + t.Slug + "/",
				Pages: t.Pages, Params: map[string]any{"tag": t}, Site: s,
			})
		}
	}
	if len(s.Tags) > 0 && opts.hasTemplate("tags.html") {
		s.Pages = append(s.Pages, &Page{Kind: KindTags, Title: "Tags", Path: "/tags/", Params: map[string]any{}, Site: s})
	}

	// Generated home and 404 pages when there is a template but no content file.
	if s.Home == nil && opts.hasTemplate("index.html") {
		s.Home = &Page{Kind: KindHome, Title: cfg.Title, Description: cfg.Description, Path: "/", Params: map[string]any{}, Site: s}
		s.Pages = append(s.Pages, s.Home)
	}
	if s.find(KindNotFound) == nil && opts.hasTemplate("404.html") {
		s.Pages = append(s.Pages, &Page{Kind: KindNotFound, Title: "Not found", Path: "/404.html", NoIndex: true, Params: map[string]any{}, Site: s})
	}

	// URLs, duplicate paths and aliases.
	seen := map[string]*Page{}
	for _, p := range s.Pages {
		p.URL = s.RelURL(p.Path)
		if q := seen[p.Path]; q != nil {
			return nil, fmt.Errorf("%s and %s both want the URL %s", describe(q), describe(p), p.Path)
		}
		seen[p.Path] = p
	}
	for _, p := range s.Pages {
		for _, a := range p.Aliases {
			from := cleanURLPath(a)
			if q := seen[from]; q != nil {
				return nil, fmt.Errorf("%s has alias %s, which is already the URL of %s", describe(p), from, describe(q))
			}
			s.aliases[from] = p.Path
		}
	}
	sort.SliceStable(s.Pages, func(i, j int) bool { return s.Pages[i].Path < s.Pages[j].Path })
	return s, nil
}

func (s *Site) collection(name string) *Collection {
	c := s.Collections[name]
	if c == nil {
		cc := s.Config.Collection(name)
		c = &Collection{Name: name, Title: cc.Title, Description: cc.Description, Config: cc}
		if c.Title == "" {
			c.Title = titleFromSlug(name)
		}
		s.Collections[name] = c
	}
	return c
}

func (s *Site) find(kind string) *Page {
	for _, p := range s.Pages {
		if p.Kind == kind {
			return p
		}
	}
	return nil
}

func describe(p *Page) string {
	if p.Source != "" {
		return "content/" + p.Source
	}
	return "the generated " + p.Kind + " page"
}

func latest(pages []*Page) time.Time {
	var t time.Time
	for _, p := range pages {
		if lm := p.LastMod(); lm.After(t) {
			t = lm
		}
	}
	return t
}

// rootLink matches href="/..." and src="/..." but not protocol-relative "//".
var rootLink = regexp.MustCompile(`(\s(?:href|src|poster|action)\s*=\s*["'])/([^/])`)

// withBasePath prefixes root-relative links in rendered content with the
// site's base path, so "[home](/)" keeps working under baseURL .../blog/.
func withBasePath(html, basePath string) string {
	if basePath == "" {
		return html
	}
	html = rootLink.ReplaceAllString(html, "${1}"+basePath+"/${2}")
	return strings.NewReplacer(`href="/"`, `href="`+basePath+`/"`, `href='/'`, `href='`+basePath+`/'`).Replace(html)
}

func renderBody(md goldmark.Markdown, p *Page, summary string) error {
	bp := p.Site.Config.BasePath
	if p.isHTML {
		p.Content = template.HTML(withBasePath(string(p.body), bp))
		p.Summary = summary
		return nil
	}
	r, err := RenderMarkdown(md, p.body)
	if err != nil {
		return err
	}
	p.Content = template.HTML(withBasePath(r.HTML, bp))
	p.Headings = r.Headings
	p.WordCount = r.WordCount
	p.Summary = r.Summary
	if summary != "" {
		p.Summary = summary
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// loadData reads data/*.yaml|yml|json from every layer (site wins). Files in
// subfolders become nested maps: data/nav/main.yaml -> .Site.Data.nav.main
func loadData(cfg *Config) (map[string]any, error) {
	out := map[string]any{}
	layers := cfg.LayerDirs()
	for i := len(layers) - 1; i >= 0; i-- {
		dir := filepath.Join(layers[i], "data")
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".yaml" && ext != ".yml" && ext != ".json" {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var v any
			if ext == ".json" {
				err = json.Unmarshal(raw, &v)
			} else {
				err = yaml.Unmarshal(raw, &v)
			}
			if err != nil {
				rel, _ := filepath.Rel(layers[i], p)
				return fmt.Errorf("%s: %w", filepath.ToSlash(rel), err)
			}
			rel, _ := filepath.Rel(dir, p)
			parts := strings.Split(filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))), "/")
			m := out
			for _, part := range parts[:len(parts)-1] {
				next, ok := m[part].(map[string]any)
				if !ok {
					next = map[string]any{}
					m[part] = next
				}
				m = next
			}
			m[parts[len(parts)-1]] = v
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
