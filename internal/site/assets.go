package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/evanw/esbuild/pkg/api"
)

// AssetDir is the output folder for processed assets.
const AssetDir = "_assets"

// SyntaxAsset is the name of the generated highlighting stylesheet.
const SyntaxAsset = "syntax.css"

var bundleExts = map[string]bool{".css": true, ".js": true, ".mjs": true, ".ts": true, ".mts": true, ".jsx": true, ".tsx": true}

var fileLoaderExts = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg", ".ico", ".bmp",
	".woff", ".woff2", ".ttf", ".otf", ".eot",
	".mp4", ".webm", ".mp3", ".ogg", ".wav", ".pdf", ".json", ".txt",
}

// Manifest maps logical asset names ("site.css", "img/logo.png") to URLs.
type Manifest map[string]string

type assetOptions struct {
	dev    bool
	minify bool
}

// buildAssets merges every layer's assets/ folder (site wins), bundles the
// top-level CSS/JS/TS entry points with esbuild, and copies every other file
// with a content hash in its name.
func buildAssets(cfg *Config, outDir string, opts assetOptions) (Manifest, error) {
	manifest := Manifest{}
	work, err := os.MkdirTemp("", "muhssg-assets-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	layers := cfg.LayerDirs()
	found := false
	for i := len(layers) - 1; i >= 0; i-- {
		src := filepath.Join(layers[i], "assets")
		if _, err := os.Stat(src); err != nil {
			continue
		}
		found = true
		if err := copyTree(src, work); err != nil {
			return nil, err
		}
	}
	if css, err := syntaxCSS(cfg); err != nil {
		return nil, err
	} else if css != "" {
		if _, err := os.Stat(filepath.Join(work, SyntaxAsset)); os.IsNotExist(err) {
			found = true
			if err := os.WriteFile(filepath.Join(work, SyntaxAsset), []byte(css), 0o644); err != nil {
				return nil, err
			}
		}
	}
	if !found {
		return manifest, nil
	}

	var entries, files []string
	err = filepath.WalkDir(work, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(work, p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(path.Base(rel), ".") {
			return nil
		}
		ext := strings.ToLower(path.Ext(rel))
		isCode := bundleExts[ext] || ext == ".cjs" || ext == ".scss"
		switch {
		case bundleExts[ext] && !strings.Contains(rel, "/") && !strings.HasPrefix(rel, "_"):
			entries = append(entries, rel)
		case isCode:
			// imported by an entry point; not published on its own
		default:
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	sort.Strings(files)

	assetOut := filepath.Join(outDir, AssetDir)
	if err := os.MkdirAll(assetOut, 0o755); err != nil {
		return nil, err
	}

	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		hashed := hashedName(rel, data)
		dst := filepath.Join(assetOut, filepath.FromSlash(hashed))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, err
		}
		manifest[rel] = cfg.BasePath + "/" + AssetDir + "/" + hashed
	}

	if len(entries) == 0 {
		return manifest, nil
	}
	loaders := map[string]api.Loader{}
	for _, ext := range fileLoaderExts {
		loaders[ext] = api.LoaderFile
	}
	loaders[".json"] = api.LoaderJSON
	loaders[".txt"] = api.LoaderText

	absEntries := make([]string, len(entries))
	for i, e := range entries {
		absEntries[i] = filepath.Join(work, filepath.FromSlash(e))
	}
	sourcemap := api.SourceMapNone
	if opts.dev {
		sourcemap = api.SourceMapInline
	}
	result := api.Build(api.BuildOptions{
		EntryPoints:       absEntries,
		AbsWorkingDir:     work,
		Outdir:            assetOut,
		Outbase:           work,
		EntryNames:        "[dir]/[name]-[hash]",
		AssetNames:        "files/[name]-[hash]",
		ChunkNames:        "chunks/[name]-[hash]",
		PublicPath:        cfg.BasePath + "/" + AssetDir,
		Bundle:            true,
		Splitting:         true,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		Target:            api.ES2020,
		MinifyWhitespace:  opts.minify,
		MinifyIdentifiers: opts.minify,
		MinifySyntax:      opts.minify,
		Sourcemap:         sourcemap,
		Loader:            loaders,
		Plugins:           []api.Plugin{rootPathsExternal},
		NodePaths:         []string{filepath.Join(cfg.Root, "node_modules")},
		Metafile:          true,
		Write:             true,
		LogLevel:          api.LogLevelSilent,
		Charset:           api.CharsetUTF8,
	})
	if len(result.Errors) > 0 {
		var b strings.Builder
		for _, m := range result.Errors {
			if m.Location != nil {
				fmt.Fprintf(&b, "\n  assets/%s:%d:%d: %s", filepath.ToSlash(m.Location.File), m.Location.Line, m.Location.Column, m.Text)
			} else {
				fmt.Fprintf(&b, "\n  %s", m.Text)
			}
		}
		return nil, fmt.Errorf("bundling assets failed:%s", b.String())
	}

	var meta struct {
		Outputs map[string]struct {
			EntryPoint string `json:"entryPoint"`
			CSSBundle  string `json:"cssBundle"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(result.Metafile), &meta); err != nil {
		return nil, fmt.Errorf("reading esbuild metafile: %w", err)
	}
	urlFor := func(out string) (string, error) {
		abs := out
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(work, filepath.FromSlash(out))
		}
		rel, err := filepath.Rel(assetOut, abs)
		if err != nil {
			return "", err
		}
		return cfg.BasePath + "/" + AssetDir + "/" + filepath.ToSlash(rel), nil
	}
	for out, info := range meta.Outputs {
		if info.EntryPoint == "" {
			continue
		}
		entry := filepath.ToSlash(info.EntryPoint)
		u, err := urlFor(out)
		if err != nil {
			return nil, err
		}
		manifest[entry] = u
		if info.CSSBundle != "" {
			cu, err := urlFor(info.CSSBundle)
			if err != nil {
				return nil, err
			}
			manifest[strings.TrimSuffix(entry, path.Ext(entry))+".css"] = cu
		}
	}
	return manifest, nil
}

// rootPathsExternal leaves imports written as site-root paths, like
// url(/fonts/x.woff2) or import "/vendor/x.js", untouched: they point at
// files in static/, not at files esbuild can bundle.
var rootPathsExternal = api.Plugin{
	Name: "muhssg-root-paths",
	Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: `^/`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			if args.Kind == api.ResolveEntryPoint {
				return api.OnResolveResult{}, nil
			}
			return api.OnResolveResult{Path: args.Path, External: true}, nil
		})
	},
}

// hashedName turns "img/logo.png" into "img/logo-1a2b3c4d.png".
func hashedName(rel string, data []byte) string {
	sum := sha256.Sum256(data)
	ext := path.Ext(rel)
	return strings.TrimSuffix(rel, ext) + "-" + hex.EncodeToString(sum[:4]) + ext
}

// syntaxCSS builds the highlighting stylesheet from the configured styles.
// With both a light and a dark style, each is scoped to its own
// prefers-color-scheme query so rules from one can never leak into the other.
func syntaxCSS(cfg *Config) (string, error) {
	h := cfg.Highlight
	if h.Disable || (h.Style == "" && h.DarkStyle == "") {
		return "", nil
	}
	return SyntaxCSS(h.Style, h.DarkStyle)
}

// SyntaxCSS returns highlighting CSS for a light style, a dark style, or both.
func SyntaxCSS(lightStyle, darkStyle string) (string, error) {
	if lightStyle == "" || darkStyle == "" {
		return StyleCSS(firstNonEmpty(lightStyle, darkStyle))
	}
	light, err := StyleCSS(lightStyle)
	if err != nil {
		return "", err
	}
	dark, err := StyleCSS(darkStyle)
	if err != nil {
		return "", err
	}
	return "@media (prefers-color-scheme: light) {\n" + light + "}\n@media (prefers-color-scheme: dark) {\n" + dark + "}\n", nil
}

// StyleCSS returns the CSS for a chroma style name.
func StyleCSS(name string) (string, error) {
	style := styles.Get(name)
	if style == nil || (style == styles.Fallback && name != styles.Fallback.Name) {
		return "", fmt.Errorf("highlight style %q not found (see `muhssg styles`)", name)
	}
	var buf bytes.Buffer
	if err := chromahtml.New(chromahtml.WithClasses(true)).WriteCSS(&buf, style); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// StyleNames lists the available highlighting styles.
func StyleNames() []string {
	names := styles.Names()
	sort.Strings(names)
	return names
}

// copyTree copies src into dst, overwriting files that already exist.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
