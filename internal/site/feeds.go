package site

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	AtomNS  string     `xml:"xmlns:atom,attr"`
	Content string     `xml:"xmlns:content,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language,omitempty"`
	Generator   string    `xml:"generator"`
	LastBuild   string    `xml:"lastBuildDate,omitempty"`
	AtomLink    atomLink  `xml:"atom:link"`
	Items       []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        rssGUID  `xml:"guid"`
	PubDate     string   `xml:"pubDate,omitempty"`
	Description string   `xml:"description,omitempty"`
	Content     *cdata   `xml:"content:encoded,omitempty"`
	Categories  []string `xml:"category,omitempty"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}

type cdata struct {
	Value string `xml:",cdata"`
}

func writeXML(path string, v any) error {
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), append(out, '\n')...), 0o644)
}

func writeFeed(s *Site, outDir string, f feedInfo) error {
	c := f.Coll
	pages := c.Pages
	if c.Config.FeedLimit > 0 && len(pages) > c.Config.FeedLimit {
		pages = pages[:c.Config.FeedLimit]
	}
	title := s.Title
	if c.Title != "" && c.Title != s.Title {
		title = c.Title + " — " + s.Title
	}
	desc := c.Description
	if desc == "" {
		desc = s.Description
	}
	link := s.AbsURL("/")
	if c.List != nil {
		link = s.AbsURL(c.List.Path)
	}
	ch := rssChannel{
		Title:       title,
		Link:        link,
		Description: desc,
		Language:    s.Language,
		Generator:   s.Generator,
		AtomLink:    atomLink{Href: s.AbsURL(f.Path), Rel: "self", Type: "application/rss+xml"},
	}
	if t := latest(pages); !t.IsZero() {
		ch.LastBuild = t.Format(time.RFC1123Z)
	}
	for _, p := range pages {
		item := rssItem{
			Title:       p.Title,
			Link:        p.Permalink(),
			GUID:        rssGUID{Value: p.Permalink(), IsPermaLink: s.BaseURL != ""},
			Description: firstNonEmpty(p.Description, p.Summary),
			Categories:  p.Tags,
		}
		if !p.Date.IsZero() {
			item.PubDate = p.Date.Format(time.RFC1123Z)
		}
		if p.Content != "" {
			item.Content = &cdata{Value: string(p.Content)}
		}
		ch.Items = append(ch.Items, item)
	}
	feed := rssFeed{Version: "2.0", AtomNS: "http://www.w3.org/2005/Atom", Content: "http://purl.org/rss/1.0/modules/content/", Channel: ch}
	return writeXML(filepath.Join(outDir, outputFile(f.Path)), feed)
}

type urlSet struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func writeSitemap(s *Site, outDir string) error {
	set := urlSet{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range s.Pages {
		if p.NoIndex || p.Draft || p.Kind == KindNotFound {
			continue
		}
		u := sitemapURL{Loc: p.Permalink()}
		if lm := p.LastMod(); !lm.IsZero() {
			u.LastMod = lm.Format("2006-01-02")
		}
		set.URLs = append(set.URLs, u)
	}
	return writeXML(filepath.Join(outDir, "sitemap.xml"), set)
}

func writeRobots(s *Site, outDir string) error {
	path := filepath.Join(outDir, "robots.txt")
	if _, err := os.Stat(path); err == nil {
		return nil // the site ships its own
	}
	body := "User-agent: *\nAllow: /\n"
	if s.BaseURL != "" {
		body += "\nSitemap: " + s.AbsURL("/sitemap.xml") + "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// writeRedirects writes an HTML stub for each redirect and appends the
// redirects to _redirects (understood by Netlify and Cloudflare Pages).
func writeRedirects(s *Site, outDir string, taken map[string]bool) error {
	all := map[string]string{}
	for from, to := range s.Config.Redirects {
		all[from] = to
	}
	for from, to := range s.aliases {
		all[from] = to
	}
	if len(all) == 0 {
		return nil
	}
	froms := sortedKeys(all)
	var lines []string
	for _, from := range froms {
		to := all[from]
		if !strings.HasPrefix(from, "/") {
			return fmt.Errorf("redirect %q: the source must start with /", from)
		}
		target := to
		if strings.HasPrefix(to, "/") {
			target = s.RelURL(cleanURLPathKeepFile(to))
		}
		src := cleanURLPath(from)
		file := outputFile(src)
		if taken[filepath.ToSlash(file)] {
			return fmt.Errorf("redirect from %s would overwrite a page at the same URL", from)
		}
		abs := target
		if strings.HasPrefix(target, "/") {
			abs = s.AbsURL(strings.TrimPrefix(target, s.Config.BasePath))
		}
		html := fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<title>Redirecting…</title>
<meta name="robots" content="noindex">
<meta http-equiv="refresh" content="0; url=%s">
<link rel="canonical" href="%s">
</head>
<body><p>Moved to <a href="%s">%s</a>.</p></body>
</html>
`, template.HTMLEscapeString(s.Language), template.HTMLEscapeString(target), template.HTMLEscapeString(abs), template.HTMLEscapeString(target), template.HTMLEscapeString(target))
		dst := filepath.Join(outDir, file)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(html), 0o644); err != nil {
			return err
		}
		pub := s.RelURL(src)
		lines = append(lines, fmt.Sprintf("%s %s 301", pub, target))
		if strings.HasSuffix(pub, "/") && pub != "/" {
			lines = append(lines, fmt.Sprintf("%s %s 301", strings.TrimSuffix(pub, "/"), target))
		}
	}
	sort.Strings(lines)
	path := filepath.Join(outDir, "_redirects")
	existing, _ := os.ReadFile(path)
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "# generated by muhssg\n" + strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// cleanURLPathKeepFile normalises a redirect target without forcing a
// trailing slash onto paths that have a query or fragment.
func cleanURLPathKeepFile(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		return cleanURLPath(p[:i]) + p[i:]
	}
	return cleanURLPath(p)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
