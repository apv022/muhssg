// Package scaffold creates new sites and content files.
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apv022/muhssg/internal/site"
)

//go:embed all:starter
var starter embed.FS

// NewSite writes the starter site into dir, which must be empty or missing.
func NewSite(dir, title string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty; pick a new folder", dir)
	}
	return fs.WalkDir(starter, "starter", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "starter"), "/")
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := starter.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "muhssg.yaml" && title != "" {
			data = []byte(strings.Replace(string(data), "title: My Site", "title: "+yamlString(title), 1))
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func yamlString(s string) string {
	if strings.ContainsAny(s, `:#'"{}[],&*?|<>=!%@`+"`") || strings.TrimSpace(s) != s {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}

// NewContent creates content/<collection>/<slug>.md as a draft dated today
// and returns its path.
func NewContent(root, collection, title string) (string, error) {
	if _, err := site.LoadConfig(root); err != nil {
		return "", err
	}
	slug := site.Slugify(title)
	if slug == "" {
		return "", fmt.Errorf("cannot make a file name from %q", title)
	}
	collection = strings.Trim(filepath.ToSlash(collection), "/")
	if collection == "" || strings.Contains(collection, "..") {
		return "", fmt.Errorf("%q is not a collection name", collection)
	}
	dir := filepath.Join(root, "content", filepath.FromSlash(collection))
	path := filepath.Join(dir, slug+".md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body := fmt.Sprintf("---\ntitle: %s\ndescription: \"\"\ndate: %s\ndraft: true\ntags: []\n---\n\n", yamlString(title), time.Now().Format("2006-01-02"))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
