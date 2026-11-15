package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is set by the main package.
var Version = "dev"

// Options control a build.
type Options struct {
	Root    string // site root (folder with muhssg.yaml)
	OutDir  string // overrides config output
	BaseURL string // overrides config baseURL (useful for preview deploys)
	Drafts  bool   // include draft pages
	Dev     bool   // development build: no minification, inline source maps
}

// Result describes a finished build.
type Result struct {
	OutDir   string
	Pages    int
	Assets   int
	Duration time.Duration
	Warnings []string
	Site     *Site
}

// Build renders the site in opts.Root.
func Build(opts Options) (*Result, error) {
	start := time.Now()
	cfg, err := LoadConfig(opts.Root)
	if err != nil {
		return nil, err
	}
	if opts.BaseURL != "" {
		if err := cfg.SetBaseURL(opts.BaseURL); err != nil {
			return nil, err
		}
	}
	if opts.OutDir != "" {
		cfg.Output = opts.OutDir
		if !filepath.IsAbs(opts.OutDir) {
			abs, err := filepath.Abs(opts.OutDir)
			if err != nil {
				return nil, err
			}
			cfg.Output = abs
		}
	}
	for _, dir := range cfg.ThemeDirs() {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("theme folder %s does not exist", dir)
		}
	}
	outDir := cfg.OutputDir()
	if err := checkOutputDir(cfg, outDir); err != nil {
		return nil, err
	}

	res := &Result{OutDir: outDir}
	if cfg.BaseURL == "" {
		res.Warnings = append(res.Warnings, "no baseURL in muhssg.yaml: canonical links, feeds and the sitemap will use relative URLs")
	}

	parent := filepath.Dir(outDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(parent, ".muhssg-build-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()

	site, err := render(cfg, tmp, opts, res)
	if err != nil {
		return nil, err
	}
	res.Site = site

	// Swap the finished build into place.
	old := ""
	if _, err := os.Stat(outDir); err == nil {
		old = outDir + ".old-" + fmt.Sprint(time.Now().UnixNano())
		if err := os.Rename(outDir, old); err != nil {
			return nil, fmt.Errorf("replacing %s: %w", outDir, err)
		}
	}
	if err := os.Rename(tmp, outDir); err != nil {
		if old != "" {
			_ = os.Rename(old, outDir)
		}
		return nil, fmt.Errorf("writing %s: %w", outDir, err)
	}
	ok = true
	if old != "" {
		_ = os.RemoveAll(old)
	}
	res.Duration = time.Since(start)
	return res, nil
}

// checkOutputDir refuses output folders that would destroy source files.
func checkOutputDir(cfg *Config, out string) error {
	out = filepath.Clean(out)
	root := filepath.Clean(cfg.Root)
	if out == root || strings.HasPrefix(root+string(filepath.Separator), out+string(filepath.Separator)) {
		return fmt.Errorf("output folder %s would contain the site itself; pick a subfolder like dist", out)
	}
	for _, layer := range cfg.LayerDirs() {
		for _, name := range []string{"content", "templates", "assets", "static", "data", "themes"} {
			src := filepath.Join(layer, name)
			if out == src || strings.HasPrefix(out+string(filepath.Separator), src+string(filepath.Separator)) {
				return fmt.Errorf("output folder %s is inside the source folder %s", out, src)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(out, ".git")); err == nil {
		return fmt.Errorf("output folder %s is a git repository; refusing to replace it", out)
	}
	return nil
}

func render(cfg *Config, out string, opts Options, res *Result) (*Site, error) {
	// 1. static/ from every layer, site last so it wins.
	layers := cfg.LayerDirs()
	for i := len(layers) - 1; i >= 0; i-- {
		dir := filepath.Join(layers[i], "static")
		if _, err := os.Stat(dir); err == nil {
			if err := copyTree(dir, out); err != nil {
				return nil, err
			}
		}
	}

	// 2. Assets.
	manifest, err := buildAssets(cfg, out, assetOptions{dev: opts.Dev, minify: !opts.Dev && boolOr(cfg.Minify, true)})
	if err != nil {
		return nil, err
	}
	res.Assets = len(manifest)

	// 3. Favicons.
	favs, err := buildFavicons(cfg, out)
	if err != nil {
		return nil, err
	}

	// 4. Content, data and templates.
	raws, files, err := readContent(filepath.Join(cfg.Root, "content"))
	if err != nil {
		return nil, err
	}
	sources, err := loadTemplateSources(cfg)
	if err != nil {
		return nil, err
	}
	hasTemplate := func(name string) bool { _, ok := sources[name]; return ok }
	site, err := buildModel(cfg, raws, modelOptions{drafts: opts.Drafts, hasTemplate: hasTemplate})
	if err != nil {
		return nil, err
	}
	site.Dev = opts.Dev
	site.Generator = "muhssg " + Version
	if site.Data, err = loadData(cfg); err != nil {
		return nil, err
	}
	images := newImageProc(cfg, out)
	env := &renderEnv{site: site, manifest: manifest, favicons: favs, images: images}
	content := &contentRenderer{site: site, md: newMarkdown(cfg), images: images}
	env.content = content
	tmpl, err := parseTemplates(sources, env.funcs())
	if err != nil {
		return nil, err
	}
	content.tmpl = tmpl
	if err := content.renderAll(); err != nil {
		return nil, err
	}

	// 5. Pages.
	taken := map[string]bool{}
	for _, p := range site.Pages {
		html, err := tmpl.render(p)
		if err != nil {
			return nil, err
		}
		file := outputFile(p.Path)
		dst := filepath.Join(out, file)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, html, 0o644); err != nil {
			return nil, err
		}
		taken[filepath.ToSlash(file)] = true
	}
	res.Pages = len(site.Pages)
	if len(site.Pages) == 0 {
		res.Warnings = append(res.Warnings, "no pages were built: add Markdown files to content/")
	}

	// 6. Files that live next to content (images in page bundles, etc.).
	// Images that went through the image pipeline are published resized and
	// without metadata; their originals are not copied.
	contentDir := filepath.Join(cfg.Root, "content")
	for _, rel := range files {
		src := filepath.Join(contentDir, filepath.FromSlash(rel))
		if images.used[src] {
			continue
		}
		if err := copyFile(src, filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			return nil, err
		}
	}

	// 7. Feeds, sitemap, robots.txt, redirects.
	for _, f := range site.feeds {
		if err := writeFeed(site, out, f); err != nil {
			return nil, err
		}
	}
	if err := writeSitemap(site, out); err != nil {
		return nil, err
	}
	if boolOr(cfg.Robots, true) {
		if err := writeRobots(site, out); err != nil {
			return nil, err
		}
	}
	if err := writeRedirects(site, out, taken); err != nil {
		return nil, err
	}
	return site, nil
}
