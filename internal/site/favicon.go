package site

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type faviconSet struct {
	ico, svg, apple, manifest bool
	png                       string // a static PNG favicon when no source is configured
	themeColor                string
}

// resolveSource finds a file named in the config, looking in the site root
// first and then in each theme.
func resolveSource(cfg *Config, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		if _, err := os.Stat(rel); err != nil {
			return "", err
		}
		return rel, nil
	}
	for _, layer := range cfg.LayerDirs() {
		p := filepath.Join(layer, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in the site or its themes", rel)
}

// buildFavicons writes favicon.ico, apple-touch-icon.png, icon-192.png,
// icon-512.png and site.webmanifest from one raster source image. With no
// source configured it only detects favicons already in static/.
func buildFavicons(cfg *Config, outDir string) (faviconSet, error) {
	fc := cfg.Favicon
	set := faviconSet{themeColor: fc.ThemeColor}
	if fc.SVG != "" {
		src, err := resolveSource(cfg, fc.SVG)
		if err != nil {
			return set, fmt.Errorf("favicon.svg: %w", err)
		}
		if err := copyFile(src, filepath.Join(outDir, "favicon.svg")); err != nil {
			return set, err
		}
		set.svg = true
	}
	if fc.Source == "" {
		exists := func(name string) bool { _, err := os.Stat(filepath.Join(outDir, name)); return err == nil }
		set.ico = exists("favicon.ico")
		set.svg = set.svg || exists("favicon.svg")
		set.apple = exists("apple-touch-icon.png")
		set.manifest = exists("site.webmanifest") || exists("manifest.webmanifest")
		if exists("favicon.png") {
			set.png = "/favicon.png"
		}
		return set, nil
	}

	src, err := resolveSource(cfg, fc.Source)
	if err != nil {
		return set, fmt.Errorf("favicon.source: %w", err)
	}
	f, err := os.Open(src)
	if err != nil {
		return set, err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return set, fmt.Errorf("favicon.source: %s is not a PNG, JPEG or GIF image (%v)", fc.Source, err)
	}
	b := img.Bounds()
	if b.Dx() < 180 || b.Dy() < 180 {
		return set, fmt.Errorf("favicon.source: %s is %dx%d; use at least 180x180 (512x512 is best)", fc.Source, b.Dx(), b.Dy())
	}

	var bg color.Color
	if fc.Background != "" {
		c, err := parseHexColor(fc.Background)
		if err != nil {
			return set, fmt.Errorf("favicon.background: %w", err)
		}
		bg = c
	}

	write := func(name string, size int, fill color.Color) ([]byte, error) {
		data, err := encodePNG(resize(img, size, fill))
		if err != nil {
			return nil, err
		}
		if name != "" {
			if err := os.WriteFile(filepath.Join(outDir, name), data, 0o644); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	if _, err := write("apple-touch-icon.png", 180, bg); err != nil {
		return set, err
	}
	if _, err := write("icon-192.png", 192, nil); err != nil {
		return set, err
	}
	if _, err := write("icon-512.png", 512, nil); err != nil {
		return set, err
	}
	var icoImages [][]byte
	for _, size := range []int{16, 32, 48} {
		data, err := write("", size, nil)
		if err != nil {
			return set, err
		}
		icoImages = append(icoImages, data)
	}
	if err := os.WriteFile(filepath.Join(outDir, "favicon.ico"), encodeICO([]int{16, 32, 48}, icoImages), 0o644); err != nil {
		return set, err
	}

	name := fc.Name
	if name == "" {
		name = cfg.Title
	}
	short := fc.ShortName
	if short == "" {
		short = name
	}
	manifest := map[string]any{
		"name":       name,
		"short_name": short,
		"icons": []map[string]string{
			{"src": cfg.BasePath + "/icon-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": cfg.BasePath + "/icon-512.png", "sizes": "512x512", "type": "image/png"},
		},
		"start_url": cfg.BasePath + "/",
		"display":   "browser",
	}
	if fc.ThemeColor != "" {
		manifest["theme_color"] = fc.ThemeColor
	}
	if fc.Background != "" {
		manifest["background_color"] = fc.Background
	}
	mj, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "site.webmanifest"), append(mj, '\n'), 0o644); err != nil {
		return set, err
	}
	set.ico, set.apple, set.manifest = true, true, true
	return set, nil
}

// html returns the <link> tags for the favicon set.
func (f faviconSet) html(basePath string) template.HTML {
	var b strings.Builder
	if f.ico {
		fmt.Fprintf(&b, `<link rel="icon" href="%s/favicon.ico" sizes="48x48">`+"\n", basePath)
	}
	if f.png != "" {
		fmt.Fprintf(&b, `<link rel="icon" href="%s%s" type="image/png">`+"\n", basePath, f.png)
	}
	if f.svg {
		fmt.Fprintf(&b, `<link rel="icon" href="%s/favicon.svg" type="image/svg+xml">`+"\n", basePath)
	}
	if f.apple {
		fmt.Fprintf(&b, `<link rel="apple-touch-icon" href="%s/apple-touch-icon.png">`+"\n", basePath)
	}
	if f.manifest {
		fmt.Fprintf(&b, `<link rel="manifest" href="%s/site.webmanifest">`+"\n", basePath)
	}
	if f.themeColor != "" {
		fmt.Fprintf(&b, `<meta name="theme-color" content="%s">`+"\n", template.HTMLEscapeString(f.themeColor))
	}
	return template.HTML(b.String())
}

// resize fits img into a size×size square, centred, optionally on a fill.
func resize(img image.Image, size int, fill color.Color) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	if fill != nil {
		draw.Draw(dst, dst.Bounds(), &image.Uniform{fill}, image.Point{}, draw.Src)
	}
	b := img.Bounds()
	w, h := size, size
	if b.Dx() > b.Dy() {
		h = size * b.Dy() / b.Dx()
	} else if b.Dy() > b.Dx() {
		w = size * b.Dx() / b.Dy()
	}
	off := image.Pt((size-w)/2, (size-h)/2)
	xdraw.CatmullRom.Scale(dst, image.Rectangle{Min: off, Max: off.Add(image.Pt(w, h))}, img, b, xdraw.Over, nil)
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeICO packs PNG images into an .ico container.
func encodeICO(sizes []int, images [][]byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, data := range images {
		dim := byte(sizes[i])
		if sizes[i] >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // colour planes
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(data)
	}
	for _, data := range images {
		buf.Write(data)
	}
	return buf.Bytes()
}

func parseHexColor(s string) (color.Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return nil, fmt.Errorf("%q is not a hex colour like #0b0b0b", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("%q is not a hex colour like #0b0b0b", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}
