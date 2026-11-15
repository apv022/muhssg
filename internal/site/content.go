package site

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
)

// contentRenderer turns page bodies into HTML: shortcodes first, then
// Markdown, then responsive images and base-path links.
type contentRenderer struct {
	site   *Site
	md     goldmark.Markdown
	tmpl   *templates
	images *imageProc
}

// renderAll renders every page's body. It runs after templates are parsed,
// because shortcodes are templates.
func (c *contentRenderer) renderAll() error {
	for _, p := range c.site.Pages {
		if p.body == nil && p.Content == "" {
			p.Summary = firstNonEmpty(p.summary, p.Summary)
			continue
		}
		if err := c.renderPage(p); err != nil {
			return fmt.Errorf("%s: %w", describe(p), err)
		}
	}
	return nil
}

func (c *contentRenderer) renderPage(p *Page) error {
	if p.isHTML {
		rendered, err := c.renderHTMLFragment(p, string(p.body), 0)
		if err != nil {
			return err
		}
		p.Content = template.HTML(withBasePath(rendered, c.site.Config.BasePath))
		p.Summary = p.summary
		return nil
	}
	src, tokens, err := c.expandShortcodes(p, string(p.body), 0)
	if err != nil {
		return err
	}
	r, err := RenderMarkdown(c.md, []byte(src))
	if err != nil {
		return err
	}
	rendered, err := c.processImages(p, r.HTML)
	if err != nil {
		return err
	}
	rendered = substituteTokens(rendered, tokens)
	p.Content = template.HTML(withBasePath(rendered, c.site.Config.BasePath))
	p.Headings = r.Headings
	p.WordCount = r.WordCount
	p.Summary = firstNonEmpty(p.summary, cleanTokens(r.Summary, tokens))
	return nil
}

// renderFragment renders Markdown with shortcodes, for a shortcode's
// InnerHTML and the markdown template function.
func (c *contentRenderer) renderFragment(p *Page, src string, depth int) (string, error) {
	expanded, tokens, err := c.expandShortcodes(p, src, depth)
	if err != nil {
		return "", err
	}
	r, err := RenderMarkdown(c.md, []byte(expanded))
	if err != nil {
		return "", err
	}
	rendered, err := c.processImages(p, r.HTML)
	if err != nil {
		return "", err
	}
	return substituteTokens(rendered, tokens), nil
}

// renderHTMLFragment expands shortcodes in an .html content file.
func (c *contentRenderer) renderHTMLFragment(p *Page, src string, depth int) (string, error) {
	expanded, tokens, err := c.expandShortcodes(p, src, depth)
	if err != nil {
		return "", err
	}
	return substituteTokens(expanded, tokens), nil
}

var mdImage = regexp.MustCompile(`<img src="([^"]+)" alt="([^"]*)"( title="[^"]*")?\s*/?>`)

// processImages makes local Markdown images responsive: resized variants in
// a srcset, width and height, lazy loading, and stripped metadata.
func (c *contentRenderer) processImages(p *Page, body string) (string, error) {
	if c.images == nil || !boolOr(c.site.Config.Images.Process, true) || !strings.Contains(body, "<img ") {
		return body, nil
	}
	var firstErr error
	out := mdImage.ReplaceAllStringFunc(body, func(m string) string {
		parts := mdImage.FindStringSubmatch(m)
		src := html.UnescapeString(parts[1])
		if strings.Contains(src, "://") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") {
			return m
		}
		if _, ok := rasterExts[strings.ToLower(pathExt(src))]; !ok {
			return m
		}
		if _, err := c.images.resolve(p, src); err != nil {
			return m // leave it; `muhssg check` reports missing files
		}
		tag, err := c.images.imgTag(p, src, html.UnescapeString(parts[2]), "", "")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		if parts[3] != "" {
			return strings.Replace(string(tag), "<img ", "<img"+parts[3]+" ", 1)
		}
		return string(tag)
	})
	return out, firstErr
}

func pathExt(s string) string {
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "."); i >= 0 && !strings.Contains(s[i:], "/") {
		return s[i:]
	}
	return ""
}

// cleanTokens removes shortcode placeholders from plain-text summaries.
func cleanTokens(s string, tokens map[string]string) string {
	for t := range tokens {
		s = strings.ReplaceAll(s, t, "")
	}
	return collapseSpace(s)
}
