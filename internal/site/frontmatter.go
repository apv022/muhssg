package site

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FrontMatter holds the known front matter fields. Anything else ends up in
// Page.Params.
type FrontMatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Summary     string   `yaml:"summary"`
	Date        Date     `yaml:"date"`
	Updated     Date     `yaml:"updated"`
	Draft       bool     `yaml:"draft"`
	Tags        []string `yaml:"tags"`
	Slug        string   `yaml:"slug"`
	Permalink   string   `yaml:"permalink"`
	Layout      string   `yaml:"layout"`
	Image       string   `yaml:"image"`
	Aliases     []string `yaml:"aliases"`
	Weight      int      `yaml:"weight"`
	NoIndex     bool     `yaml:"noindex"`
	TOC         bool     `yaml:"toc"`
}

var knownFrontMatter = map[string]bool{
	"title": true, "description": true, "summary": true, "date": true, "updated": true,
	"draft": true, "tags": true, "slug": true, "permalink": true, "layout": true,
	"image": true, "aliases": true, "weight": true, "noindex": true, "toc": true,
}

// Date is a time.Time that accepts the date formats people actually write.
type Date struct{ time.Time }

var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// ParseDate parses a date in any of the accepted layouts.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date (use YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS)", s)
}

func (d *Date) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: date must be a single value", n.Line)
	}
	if n.Value == "" || n.Tag == "!!null" {
		d.Time = time.Time{}
		return nil
	}
	t, err := ParseDate(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	d.Time = t
	return nil
}

// SplitFrontMatter separates YAML front matter from the body. It accepts
// LF or CRLF line endings, a UTF-8 BOM, and a closing delimiter at the very
// end of the file. hasFM is false when the file does not start with "---".
func SplitFrontMatter(src []byte) (fm, body []byte, hasFM bool, err error) {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.Equal(bytes.TrimRight(src, " \t"), []byte("---")) {
		return nil, src, false, nil
	}
	nl := bytes.IndexByte(src, '\n')
	if nl < 0 {
		return nil, nil, true, fmt.Errorf("front matter starts with --- but is never closed")
	}
	rest := src[nl+1:]
	pos := 0
	for pos <= len(rest) {
		end := bytes.IndexByte(rest[pos:], '\n')
		var line []byte
		if end < 0 {
			line = rest[pos:]
		} else {
			line = rest[pos : pos+end]
		}
		if strings.TrimRight(string(line), " \t") == "---" {
			fm = rest[:pos]
			if end < 0 {
				body = nil
			} else {
				body = rest[pos+end+1:]
			}
			return fm, body, true, nil
		}
		if end < 0 {
			break
		}
		pos += end + 1
	}
	return nil, nil, true, fmt.Errorf("front matter starts with --- but is never closed")
}

// ParseFrontMatter decodes front matter into the known fields plus a map of
// the remaining ones.
func ParseFrontMatter(fm []byte) (FrontMatter, map[string]any, error) {
	var meta FrontMatter
	params := map[string]any{}
	if len(bytes.TrimSpace(fm)) == 0 {
		return meta, params, nil
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return meta, nil, err
	}
	all := map[string]any{}
	if err := yaml.Unmarshal(fm, &all); err != nil {
		return meta, nil, err
	}
	for k, v := range all {
		if !knownFrontMatter[k] {
			params[k] = v
		}
	}
	return meta, params, nil
}
