package site

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Problem is one issue found by Check.
type Problem struct {
	File    string // output file, relative to the output folder
	Message string
}

func (p Problem) String() string { return p.File + ": " + p.Message }

var (
	linkAttr = regexp.MustCompile(`(?i)\s(?:href|src)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	idAttr   = regexp.MustCompile(`(?i)\s(?:id|name)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	srcset   = regexp.MustCompile(`(?i)\ssrcset\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// Check scans a built site for broken internal links and anchors, and for
// pages missing a title or description.
func Check(outDir string, cfg *Config) ([]Problem, error) {
	var htmlFiles []string
	err := filepath.WalkDir(outDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			rel, _ := filepath.Rel(outDir, p)
			htmlFiles = append(htmlFiles, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(htmlFiles)

	ids := map[string]map[string]bool{}
	idsFor := func(rel string) map[string]bool {
		if m, ok := ids[rel]; ok {
			return m
		}
		m := map[string]bool{}
		if raw, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rel))); err == nil {
			for _, match := range idAttr.FindAllStringSubmatch(string(raw), -1) {
				m[match[1]+match[2]] = true
			}
		}
		ids[rel] = m
		return m
	}

	var problems []Problem
	for _, rel := range htmlFiles {
		raw, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		html := string(raw)
		if strings.Contains(html, `http-equiv="refresh"`) {
			continue // redirect stub
		}
		var links []string
		for _, m := range linkAttr.FindAllStringSubmatch(html, -1) {
			links = append(links, m[1]+m[2])
		}
		for _, m := range srcset.FindAllStringSubmatch(html, -1) {
			for _, part := range strings.Split(m[1]+m[2], ",") {
				if f := strings.Fields(part); len(f) > 0 {
					links = append(links, f[0])
				}
			}
		}
		seen := map[string]bool{}
		for _, link := range links {
			if seen[link] {
				continue
			}
			seen[link] = true
			if msg := checkLink(outDir, cfg, rel, link, idsFor); msg != "" {
				problems = append(problems, Problem{File: rel, Message: msg})
			}
		}
		if !strings.Contains(strings.ToLower(html), "<title>") || strings.Contains(strings.ToLower(html), "<title></title>") {
			problems = append(problems, Problem{File: rel, Message: "page has no <title>"})
		}
	}
	return problems, nil
}

func checkLink(outDir string, cfg *Config, from, link string, idsFor func(string) map[string]bool) string {
	link = strings.TrimSpace(link)
	lower := strings.ToLower(link)
	if link == "" || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") {
		return ""
	}
	if cfg.BaseURL != "" && strings.HasPrefix(link, cfg.BaseURL) {
		link = strings.TrimPrefix(link, cfg.Origin)
	} else if strings.Contains(link, "://") || strings.HasPrefix(link, "//") {
		return "" // external
	}
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Sprintf("malformed link %q", link)
	}
	target := u.Path
	if target == "" {
		target = "/" + from
	} else if !strings.HasPrefix(target, "/") {
		target = path.Join("/", path.Dir(from), target)
		if strings.HasSuffix(u.Path, "/") {
			target += "/"
		}
	} else {
		if cfg.BasePath != "" {
			if !strings.HasPrefix(target, cfg.BasePath+"/") && target != cfg.BasePath {
				return fmt.Sprintf("link %s is missing the base path %s", link, cfg.BasePath)
			}
			target = strings.TrimPrefix(target, cfg.BasePath)
		}
	}
	file, ok := resolveOutput(outDir, target)
	if !ok {
		return fmt.Sprintf("broken link %s", link)
	}
	if u.Fragment != "" && strings.HasSuffix(file, ".html") {
		if !idsFor(file)[u.Fragment] {
			return fmt.Sprintf("link %s points to #%s, which does not exist on that page", link, u.Fragment)
		}
	}
	return ""
}

// resolveOutput finds the file a site path is served from.
func resolveOutput(outDir, p string) (string, bool) {
	p = strings.TrimPrefix(p, "/")
	try := []string{}
	if p == "" || strings.HasSuffix(p, "/") {
		try = append(try, p+"index.html")
	} else {
		try = append(try, p, p+"/index.html", p+".html")
	}
	for _, t := range try {
		if st, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(t))); err == nil && !st.IsDir() {
			return t, true
		}
	}
	return "", false
}
