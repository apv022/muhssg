package site

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Page kinds.
const (
	KindHome     = "home"
	KindPage     = "page"   // standalone page, e.g. content/about.md
	KindSingle   = "single" // an entry in a collection, e.g. content/writing/hello.md
	KindList     = "list"   // a collection's index, e.g. /writing/
	KindNotFound = "404"
	KindTag      = "tag"
	KindTags     = "tags"
)

// Page is what templates receive as ".".
type Page struct {
	Kind        string
	Title       string
	Description string
	Section     string // collection name, "" for root pages
	Slug        string
	Path        string // site-relative path, e.g. "/writing/hello/"; used for output files
	URL         string // Path with the base path prepended; use this in links
	Date        time.Time
	Updated     time.Time
	Draft       bool
	Tags        []string
	Aliases     []string
	Weight      int
	NoIndex     bool
	ShowTOC     bool
	Layout      string
	Image       string
	Params      map[string]any
	Content     template.HTML
	Summary     string
	Headings    []Heading
	WordCount   int
	Source      string // path relative to content/, "" for generated pages

	Pages []*Page // children: entries of a list page, pages of a tag
	Prev  *Page   // previous entry in the collection's order
	Next  *Page   // next entry in the collection's order
	Site  *Site

	body   []byte
	isHTML bool
}

// Permalink is the absolute URL of the page (needs baseURL).
func (p *Page) Permalink() string { return p.Site.AbsURL(p.Path) }

// ReadingTime is an estimate in whole minutes (at least 1).
func (p *Page) ReadingTime() int {
	m := (p.WordCount + 219) / 220
	if m < 1 {
		return 1
	}
	return m
}

func (p *Page) IsHome() bool   { return p.Kind == KindHome }
func (p *Page) IsList() bool   { return p.Kind == KindList }
func (p *Page) IsSingle() bool { return p.Kind == KindSingle }
func (p *Page) HasDate() bool  { return !p.Date.IsZero() }

// LastMod is Updated if set, otherwise Date.
func (p *Page) LastMod() time.Time {
	if !p.Updated.IsZero() {
		return p.Updated
	}
	return p.Date
}

// TOC returns the page's h2–h4 headings.
func (p *Page) TOC() []Heading {
	var out []Heading
	for _, h := range p.Headings {
		if h.Level >= 2 && h.Level <= 4 && h.ID != "" {
			out = append(out, h)
		}
	}
	return out
}

// Collection is one folder under content/.
type Collection struct {
	Name        string
	Title       string
	Description string
	Pages       []*Page // sorted, drafts excluded unless building with drafts
	List        *Page   // the list page, or nil if the collection has none
	Config      CollectionConfig
}

// Tag groups pages that share a tag.
type Tag struct {
	Name  string
	Slug  string
	URL   string
	Pages []*Page
}

var nonSlug = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Slugify turns "Hello, World!" into "hello-world".
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func isContentFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".html":
		return true
	}
	return false
}

// rawPage is a content file read from disk before URLs are assigned.
type rawPage struct {
	rel     string // slash path relative to content/, e.g. "writing/hello.md"
	meta    FrontMatter
	params  map[string]any
	body    []byte
	isHTML  bool
	isIndex bool // _index.md: metadata for a collection's list page
}

// readContent walks content/ and returns content files and other files
// (which are copied to the output as-is).
func readContent(dir string) (pages []rawPage, files []string, err error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil, nil
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if p != dir && (strings.HasPrefix(name, ".") || (strings.HasPrefix(name, "_") && name != "_index.md")) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !isContentFile(name) {
			files = append(files, rel)
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fm, body, _, err := SplitFrontMatter(src)
		if err != nil {
			return fmt.Errorf("content/%s: %w", rel, err)
		}
		meta, params, err := ParseFrontMatter(fm)
		if err != nil {
			return fmt.Errorf("content/%s: front matter: %w", rel, err)
		}
		pages = append(pages, rawPage{
			rel:     rel,
			meta:    meta,
			params:  params,
			body:    body,
			isHTML:  strings.EqualFold(path.Ext(rel), ".html"),
			isIndex: path.Base(rel) == "_index.md",
		})
		return nil
	})
	return pages, files, err
}

// expandPermalink fills a pattern like "/:section/:year/:slug/".
func expandPermalink(pattern string, p *Page, filePath string) (string, error) {
	needsDate := strings.Contains(pattern, ":year") || strings.Contains(pattern, ":month") || strings.Contains(pattern, ":day")
	if needsDate && p.Date.IsZero() {
		return "", fmt.Errorf("permalink %q needs a date, but the page has none", pattern)
	}
	r := strings.NewReplacer(
		":section", p.Section,
		":slug", p.Slug,
		":filepath", filePath,
		":year", fmt.Sprintf("%04d", p.Date.Year()),
		":month", fmt.Sprintf("%02d", int(p.Date.Month())),
		":day", fmt.Sprintf("%02d", p.Date.Day()),
	)
	return cleanURLPath(r.Replace(pattern)), nil
}

// cleanURLPath makes sure a path starts with "/" and, unless it names a
// file (has an extension), ends with "/".
func cleanURLPath(p string) string {
	if p == "" {
		return "/"
	}
	trailing := strings.HasSuffix(p, "/")
	p = path.Clean("/" + p)
	if p == "/" {
		return p
	}
	if path.Ext(p) == "" || trailing {
		p += "/"
	}
	return p
}

// outputFile maps a site path to a file path relative to the output dir.
func outputFile(p string) string {
	if strings.HasSuffix(p, "/") {
		return filepath.FromSlash(strings.TrimPrefix(p, "/") + "index.html")
	}
	return filepath.FromSlash(strings.TrimPrefix(p, "/"))
}

func sortPages(pages []*Page, by, order string) {
	less := func(a, b *Page) bool {
		switch by {
		case "title":
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case "weight":
			if a.Weight != b.Weight {
				return a.Weight < b.Weight
			}
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		default:
			if !a.Date.Equal(b.Date) {
				return a.Date.Before(b.Date)
			}
			return a.Path < b.Path
		}
	}
	sort.SliceStable(pages, func(i, j int) bool {
		if order == "desc" {
			return less(pages[j], pages[i])
		}
		return less(pages[i], pages[j])
	})
}

// titleFromSlug turns "hello-world" into "Hello world" as a fallback title.
func titleFromSlug(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}
