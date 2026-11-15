package site

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFiles are the file names muhssg looks for in a site's root, in order.
var ConfigFiles = []string{"muhssg.yaml", "muhssg.yml"}

// Config is the contents of muhssg.yaml.
type Config struct {
	Title       string                      `yaml:"title"`
	BaseURL     string                      `yaml:"baseURL"`
	Language    string                      `yaml:"language"`
	Description string                      `yaml:"description"`
	Author      string                      `yaml:"author"`
	Image       string                      `yaml:"image"`
	Theme       StringList                  `yaml:"theme"`
	Output      string                      `yaml:"output"`
	Permalinks  map[string]string           `yaml:"permalinks"`
	Collections map[string]CollectionConfig `yaml:"collections"`
	Redirects   map[string]string           `yaml:"redirects"`
	Favicon     FaviconConfig               `yaml:"favicon"`
	Highlight   HighlightConfig             `yaml:"highlight"`
	Markdown    MarkdownConfig              `yaml:"markdown"`
	Minify      *bool                       `yaml:"minify"`
	Robots      *bool                       `yaml:"robots"`
	Params      map[string]any              `yaml:"params"`

	// Derived values, filled in by LoadConfig.
	Root     string `yaml:"-"`
	BasePath string `yaml:"-"` // path part of BaseURL without trailing slash, e.g. "/blog"
	Origin   string `yaml:"-"` // scheme+host of BaseURL, e.g. "https://apv.cool"
}

// CollectionConfig configures one folder under content/.
type CollectionConfig struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Sort        string `yaml:"sort"`  // date (default), title, weight
	Order       string `yaml:"order"` // asc or desc; default desc for date, asc otherwise
	Permalink   string `yaml:"permalink"`
	Feed        bool   `yaml:"feed"`
	FeedPath    string `yaml:"feedPath"`
	FeedLimit   int    `yaml:"feedLimit"`
}

// FaviconConfig turns one source image into a full favicon set.
type FaviconConfig struct {
	Source     string `yaml:"source"`     // raster image (PNG/JPEG), ideally 512x512 or larger
	SVG        string `yaml:"svg"`        // optional SVG icon served as /favicon.svg
	Background string `yaml:"background"` // fill behind the Apple touch icon, e.g. "#0b0b0b"
	ThemeColor string `yaml:"themeColor"`
	Name       string `yaml:"name"`
	ShortName  string `yaml:"shortName"`
}

// HighlightConfig controls syntax highlighting of fenced code blocks.
type HighlightConfig struct {
	Style       string `yaml:"style"`     // chroma style for light mode (or the only style)
	DarkStyle   string `yaml:"darkStyle"` // optional chroma style used under prefers-color-scheme: dark
	LineNumbers bool   `yaml:"lineNumbers"`
	Disable     bool   `yaml:"disable"`
}

// MarkdownConfig controls the Markdown renderer.
type MarkdownConfig struct {
	Unsafe      *bool `yaml:"unsafe"`      // allow raw HTML in Markdown (default true)
	Typographer *bool `yaml:"typographer"` // smart quotes and dashes (default true)
}

// StringList accepts either a single string or a list of strings in YAML.
type StringList []string

func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value == "" {
			*s = nil
			return nil
		}
		*s = StringList{n.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := n.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	}
	return fmt.Errorf("line %d: expected a string or a list of strings", n.Line)
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// FindConfig returns the config file path in root, or "" if there is none.
func FindConfig(root string) string {
	for _, name := range ConfigFiles {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoadConfig reads and validates the site config in root.
func LoadConfig(root string) (*Config, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path := FindConfig(abs)
	if path == "" {
		return nil, fmt.Errorf("no muhssg.yaml in %s (run `muhssg new site <dir>` to start one)", abs)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
	}
	cfg.Root = abs
	if err := cfg.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return cfg, nil
}

func (c *Config) normalize() error {
	if c.Language == "" {
		c.Language = "en"
	}
	if c.Output == "" {
		c.Output = "dist"
	}
	if c.Collections == nil {
		c.Collections = map[string]CollectionConfig{}
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	for name, cc := range c.Collections {
		switch cc.Sort {
		case "", "date", "title", "weight":
		default:
			return fmt.Errorf("collections.%s.sort: %q is not one of date, title, weight", name, cc.Sort)
		}
		switch cc.Order {
		case "", "asc", "desc":
		default:
			return fmt.Errorf("collections.%s.order: %q is not asc or desc", name, cc.Order)
		}
	}
	return c.SetBaseURL(c.BaseURL)
}

// SetBaseURL parses and stores the base URL. An empty value is allowed
// (feeds and the sitemap then use relative links and print a warning).
func (c *Config) SetBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	c.BaseURL, c.BasePath, c.Origin = "", "", ""
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("baseURL: %q must be an absolute URL like https://example.com", raw)
	}
	c.Origin = u.Scheme + "://" + u.Host
	c.BasePath = strings.TrimSuffix(u.Path, "/")
	c.BaseURL = c.Origin + c.BasePath
	return nil
}

// ThemeDirs returns the theme directories in lookup order (first wins).
func (c *Config) ThemeDirs() []string {
	var dirs []string
	for _, t := range c.Theme {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if filepath.IsAbs(t) {
			dirs = append(dirs, t)
		} else if strings.ContainsAny(t, `/\`) {
			dirs = append(dirs, filepath.Join(c.Root, t))
		} else {
			dirs = append(dirs, filepath.Join(c.Root, "themes", t))
		}
	}
	return dirs
}

// LayerDirs returns the directories that can hold templates/, assets/ and
// static/, in lookup order: the site itself first, then each theme.
func (c *Config) LayerDirs() []string {
	return append([]string{c.Root}, c.ThemeDirs()...)
}

// OutputDir returns the absolute output directory.
func (c *Config) OutputDir() string {
	if filepath.IsAbs(c.Output) {
		return filepath.Clean(c.Output)
	}
	return filepath.Join(c.Root, c.Output)
}

// Collection returns the config for a collection, with defaults applied.
func (c *Config) Collection(name string) CollectionConfig {
	cc := c.Collections[name]
	if cc.Sort == "" {
		cc.Sort = "date"
	}
	if cc.Order == "" {
		if cc.Sort == "date" {
			cc.Order = "desc"
		} else {
			cc.Order = "asc"
		}
	}
	if cc.Permalink == "" {
		cc.Permalink = c.Permalinks[name]
	}
	if cc.Feed && cc.FeedPath == "" {
		cc.FeedPath = "/" + name + "/rss.xml"
	}
	return cc
}
