package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// ImagesConfig controls responsive image generation.
type ImagesConfig struct {
	Widths  []int  `yaml:"widths"`  // srcset widths; default 480, 800, 1200, 1600
	Quality int    `yaml:"quality"` // JPEG quality, 1-100; default 82
	Sizes   string `yaml:"sizes"`   // default sizes attribute; default "100vw"
	Process *bool  `yaml:"process"` // make Markdown images responsive; default true
}

func (c ImagesConfig) widths() []int {
	if len(c.Widths) > 0 {
		w := append([]int(nil), c.Widths...)
		sort.Ints(w)
		return w
	}
	return []int{480, 800, 1200, 1600}
}

func (c ImagesConfig) quality() int {
	if c.Quality < 1 || c.Quality > 100 {
		return 82
	}
	return c.Quality
}

// Image is a published image. Width and Height are 0 for SVGs.
type Image struct {
	URL    string
	Width  int
	Height int
	proc   *imageProc
	src    *sourceImage
}

// Resize returns a copy at most width pixels wide (never upscaled).
func (i *Image) Resize(width int) (*Image, error) {
	if i.src.decoded == nil {
		return i, nil // SVG or GIF: published as-is
	}
	return i.proc.variant(i.src, width)
}

type sourceImage struct {
	path    string // absolute path on disk
	rel     string // name used for the output file
	data    []byte
	hash    string
	format  string // jpeg, png, gif, svg
	decoded image.Image
	width   int
	height  int
}

type imageProc struct {
	cfg      *Config
	outDir   string
	cacheDir string
	sources  map[string]*sourceImage
	variants map[string]*Image
	used     map[string]bool // absolute source paths that were processed
}

func newImageProc(cfg *Config, outDir string) *imageProc {
	cache := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(dir, "muhssg", "images")
	}
	return &imageProc{cfg: cfg, outDir: outDir, cacheDir: cache, sources: map[string]*sourceImage{}, variants: map[string]*Image{}, used: map[string]bool{}}
}

var rasterExts = map[string]string{".jpg": "jpeg", ".jpeg": "jpeg", ".png": "png", ".gif": "gif", ".svg": "svg"}

// resolve finds an image: relative to the page's content folder first (for
// page bundles), then in assets/, content/ and static/ of the site and themes.
func (ip *imageProc) resolve(p *Page, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("image path is empty")
	}
	if strings.Contains(ref, "://") || strings.HasPrefix(ref, "//") || strings.HasPrefix(ref, "data:") {
		return "", fmt.Errorf("%q is a remote image; only local images can be processed", ref)
	}
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(ref)), "/")
	var tries []string
	if p != nil && p.Source != "" && !strings.HasPrefix(ref, "/") {
		tries = append(tries, filepath.Join(ip.cfg.Root, "content", filepath.FromSlash(path.Join(path.Dir(p.Source), clean))))
	}
	for _, layer := range ip.cfg.LayerDirs() {
		for _, dir := range []string{"assets", "content", "static"} {
			tries = append(tries, filepath.Join(layer, dir, filepath.FromSlash(clean)))
		}
	}
	for _, t := range tries {
		if st, err := os.Stat(t); err == nil && !st.IsDir() {
			return t, nil
		}
	}
	return "", fmt.Errorf("image %q not found (looked next to the page, then in assets/, content/ and static/)", ref)
}

func (ip *imageProc) load(abs string) (*sourceImage, error) {
	if s, ok := ip.sources[abs]; ok {
		return s, nil
	}
	format, ok := rasterExts[strings.ToLower(filepath.Ext(abs))]
	if !ok {
		return nil, fmt.Errorf("%s: only JPEG, PNG, GIF and SVG images are supported", filepath.Base(abs))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	rel := filepath.Base(abs)
	for _, layer := range ip.cfg.LayerDirs() {
		for _, dir := range []string{"assets", "content", "static"} {
			if r, err := filepath.Rel(filepath.Join(layer, dir), abs); err == nil && !strings.HasPrefix(r, "..") {
				rel = filepath.ToSlash(r)
			}
		}
	}
	s := &sourceImage{path: abs, rel: rel, data: data, hash: hex.EncodeToString(sum[:]), format: format}
	switch format {
	case "jpeg", "png":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid %s image (%v)", filepath.Base(abs), strings.ToUpper(format), err)
		}
		if format == "jpeg" {
			img = applyOrientation(img, jpegOrientation(data))
		}
		s.decoded = img
		s.width, s.height = img.Bounds().Dx(), img.Bounds().Dy()
	case "gif":
		cfg, err := gif.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid GIF image (%v)", filepath.Base(abs), err)
		}
		s.width, s.height = cfg.Width, cfg.Height
	}
	ip.sources[abs] = s
	ip.used[abs] = true
	return s, nil
}

// original publishes the image at full size (re-encoded, which strips
// metadata such as GPS location) and returns it.
func (ip *imageProc) original(p *Page, ref string) (*Image, error) {
	abs, err := ip.resolve(p, ref)
	if err != nil {
		return nil, err
	}
	s, err := ip.load(abs)
	if err != nil {
		return nil, err
	}
	if s.decoded == nil { // SVG and GIF are copied untouched
		key := s.hash + "-orig"
		if img, ok := ip.variants[key]; ok {
			return img, nil
		}
		name := hashedName(s.rel, s.data)
		if err := ip.write(name, s.data); err != nil {
			return nil, err
		}
		img := &Image{URL: ip.cfg.BasePath + "/" + AssetDir + "/images/" + name, Width: s.width, Height: s.height, proc: ip, src: s}
		ip.variants[key] = img
		return img, nil
	}
	return ip.variant(s, s.width)
}

func (ip *imageProc) variant(s *sourceImage, width int) (*Image, error) {
	if width <= 0 {
		return nil, fmt.Errorf("resize width must be positive, got %d", width)
	}
	if width > s.width {
		width = s.width
	}
	q := ip.cfg.Images.quality()
	key := fmt.Sprintf("%s-%d-q%d", s.hash, width, q)
	if img, ok := ip.variants[key]; ok {
		return img, nil
	}
	height := (s.height*width + s.width/2) / s.width
	if height < 1 {
		height = 1
	}
	ext := ".png"
	if s.format == "jpeg" {
		ext = ".jpg"
	}
	stem := strings.TrimSuffix(s.rel, path.Ext(s.rel))
	name := fmt.Sprintf("%s-%s-%d%s", stem, s.hash[:8], width, ext)

	data, err := ip.cached(key + ext)
	if err != nil {
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		if s.format == "jpeg" {
			draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
		}
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), s.decoded, s.decoded.Bounds(), xdraw.Over, nil)
		var buf bytes.Buffer
		if s.format == "jpeg" {
			err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: q})
		} else {
			err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, dst)
		}
		if err != nil {
			return nil, err
		}
		data = buf.Bytes()
		ip.storeCache(key+ext, data)
	}
	if err := ip.write(name, data); err != nil {
		return nil, err
	}
	img := &Image{URL: ip.cfg.BasePath + "/" + AssetDir + "/images/" + name, Width: width, Height: height, proc: ip, src: s}
	ip.variants[key] = img
	return img, nil
}

func (ip *imageProc) write(name string, data []byte) error {
	dst := filepath.Join(ip.outDir, AssetDir, "images", filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func (ip *imageProc) cached(key string) ([]byte, error) {
	if ip.cacheDir == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(ip.cacheDir, key))
}

func (ip *imageProc) storeCache(key string, data []byte) {
	if ip.cacheDir == "" {
		return
	}
	if err := os.MkdirAll(ip.cacheDir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(ip.cacheDir, key), data, 0o644)
	}
}

// imgTag builds a responsive <img> with srcset, width/height and lazy loading.
func (ip *imageProc) imgTag(p *Page, ref, alt, sizes, class string) (template.HTML, error) {
	orig, err := ip.original(p, ref)
	if err != nil {
		return "", err
	}
	esc := template.HTMLEscapeString
	attrs := func(src *Image, srcset string) string {
		var b strings.Builder
		fmt.Fprintf(&b, `<img src="%s" alt="%s"`, esc(src.URL), esc(alt))
		if srcset != "" {
			if sizes == "" {
				sizes = ip.cfg.Images.Sizes
			}
			if sizes == "" {
				sizes = "100vw"
			}
			fmt.Fprintf(&b, ` srcset="%s" sizes="%s"`, esc(srcset), esc(sizes))
		}
		if src.Width > 0 && src.Height > 0 {
			fmt.Fprintf(&b, ` width="%d" height="%d"`, src.Width, src.Height)
		}
		if class != "" {
			fmt.Fprintf(&b, ` class="%s"`, esc(class))
		}
		b.WriteString(` loading="lazy" decoding="async">`)
		return b.String()
	}
	if orig.src.decoded == nil {
		return template.HTML(attrs(orig, "")), nil
	}
	var set []string
	var largest *Image
	for _, w := range ip.cfg.Images.widths() {
		if w >= orig.Width {
			break
		}
		v, err := ip.variant(orig.src, w)
		if err != nil {
			return "", err
		}
		set = append(set, fmt.Sprintf("%s %dw", v.URL, v.Width))
		largest = v
	}
	set = append(set, fmt.Sprintf("%s %dw", orig.URL, orig.Width))
	src := orig
	if largest != nil && orig.Width > ip.cfg.Images.widths()[len(ip.cfg.Images.widths())-1] {
		src = largest // default to the largest configured width, not a huge original
	}
	if len(set) == 1 {
		return template.HTML(attrs(src, "")), nil
	}
	return template.HTML(attrs(src, strings.Join(set, ", "))), nil
}

// jpegOrientation reads the EXIF orientation tag (1-8) from a JPEG, or 1.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation rotates/flips an image according to an EXIF orientation.
func applyOrientation(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
