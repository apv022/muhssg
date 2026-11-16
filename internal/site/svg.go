package site

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// svgInliner implements the svg template function: it reads an SVG file at
// build time and returns a cleaned-up <svg> element that can be styled with
// CSS (for example fill="currentColor" following the text colour).
type svgInliner struct {
	images *imageProc // for file lookup (page bundle, assets/, content/, static/)
	cache  map[string][]byte
	uses   int // makes ids unique across every inlined copy in a build
}

func newSVGInliner(images *imageProc) *svgInliner {
	return &svgInliner{images: images, cache: map[string][]byte{}}
}

// svgOptions are the optional settings: {{ svg "x.svg" (dict "class" "logo" "title" "Name") }}
type svgOptions struct {
	class, title, width, height string
}

func parseSVGOptions(m map[string]any) (svgOptions, error) {
	var o svgOptions
	for k, v := range m {
		s := fmt.Sprint(v)
		switch k {
		case "class":
			o.class = s
		case "title":
			o.title = s
		case "width":
			o.width = s
		case "height":
			o.height = s
		default:
			return o, fmt.Errorf("svg: unknown option %q (use class, title, width or height)", k)
		}
	}
	return o, nil
}

// inline returns the cleaned SVG markup for ref.
func (s *svgInliner) inline(p *Page, ref string, opts svgOptions) (template.HTML, error) {
	if !strings.EqualFold(filepath.Ext(ref), ".svg") {
		return "", fmt.Errorf("svg: %q is not an .svg file", ref)
	}
	abs, err := s.images.resolve(p, ref)
	if err != nil {
		return "", fmt.Errorf("svg: %w", err)
	}
	data, ok := s.cache[abs]
	if !ok {
		if data, err = os.ReadFile(abs); err != nil {
			return "", err
		}
		s.cache[abs] = data
	}
	s.uses++
	out, err := cleanSVG(data, fmt.Sprintf("svg%d-", s.uses), opts)
	if err != nil {
		return "", fmt.Errorf("svg: %s: %w", ref, err)
	}
	return template.HTML(out), nil
}

var (
	urlRef     = regexp.MustCompile(`url\(\s*['"]?#([^'")\s]+)['"]?\s*\)`)
	idRefAttrs = map[string]bool{"aria-labelledby": true, "aria-describedby": true, "aria-controls": true, "aria-owns": true, "aria-activedescendant": true, "aria-details": true, "aria-errormessage": true, "aria-flowto": true}
)

// cleanSVG parses an SVG document and re-serialises its root <svg> element:
// no XML prolog, comments, doctype, <script>, event handlers or javascript:
// links; ids are prefixed and references to them updated; class, size and an
// accessible name are applied to the root.
func cleanSVG(data []byte, prefix string, opts svgOptions) (string, error) {
	// First pass: collect ids so references can be rewritten.
	ids := map[string]bool{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	sawSVG := false
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("not valid SVG/XML: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			if !sawSVG {
				if se.Name.Local != "svg" {
					return "", fmt.Errorf("the root element is <%s>, not <svg>", se.Name.Local)
				}
				sawSVG = true
			}
			for _, a := range se.Attr {
				if a.Name.Local == "id" && a.Name.Space == "" {
					ids[a.Value] = true
				}
			}
		}
	}
	if !sawSVG {
		return "", fmt.Errorf("no <svg> element found")
	}
	rename := func(id string) string {
		if ids[id] {
			return prefix + id
		}
		return id
	}
	fixRefs := func(v string) string {
		return urlRef.ReplaceAllStringFunc(v, func(m string) string {
			id := urlRef.FindStringSubmatch(m)[1]
			return "url(#" + rename(id) + ")"
		})
	}

	var b strings.Builder
	dec = xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	depth := 0
	skip := 0 // >0 while inside a <script> (or a <title> being replaced)
	inStyle := false
	titleID := prefix + "title"
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("not valid SVG/XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := qname(t.Name)
			if skip > 0 {
				skip++
				continue
			}
			if strings.EqualFold(t.Name.Local, "script") || strings.EqualFold(t.Name.Local, "foreignObject") ||
				(depth == 1 && opts.title != "" && t.Name.Local == "title") {
				skip = 1
				continue
			}
			depth++
			b.WriteString("<" + name)
			var class string
			for _, a := range t.Attr {
				an := qname(a.Name)
				lower := strings.ToLower(a.Name.Local)
				val := a.Value
				switch {
				case strings.HasPrefix(lower, "on"):
					continue // event handlers
				case (lower == "href") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(val)), "javascript:"):
					continue
				case lower == "href" && strings.HasPrefix(val, "#"):
					val = "#" + rename(val[1:])
				case a.Name.Local == "id" && a.Name.Space == "":
					val = rename(val)
				case idRefAttrs[lower]:
					parts := strings.Fields(val)
					for i, p := range parts {
						parts[i] = rename(p)
					}
					val = strings.Join(parts, " ")
				default:
					val = fixRefs(val)
				}
				if depth == 1 {
					switch an {
					case "class":
						class = val
						continue
					case "width":
						if opts.width != "" {
							continue
						}
					case "height":
						if opts.height != "" {
							continue
						}
					case "role", "aria-hidden", "aria-labelledby", "aria-label", "focusable":
						continue // set below
					}
				}
				fmt.Fprintf(&b, ` %s="%s"`, an, escapeAttr(val))
			}
			if depth == 1 {
				if c := strings.TrimSpace(strings.TrimSpace(class) + " " + opts.class); c != "" {
					fmt.Fprintf(&b, ` class="%s"`, escapeAttr(c))
				}
				if opts.width != "" {
					fmt.Fprintf(&b, ` width="%s"`, escapeAttr(opts.width))
				}
				if opts.height != "" {
					fmt.Fprintf(&b, ` height="%s"`, escapeAttr(opts.height))
				}
				if opts.title != "" {
					fmt.Fprintf(&b, ` role="img" aria-labelledby="%s"`, titleID)
				} else {
					b.WriteString(` aria-hidden="true" focusable="false"`)
				}
				b.WriteString(">")
				if opts.title != "" {
					fmt.Fprintf(&b, `<title id="%s">%s</title>`, titleID, template.HTMLEscapeString(opts.title))
				}
				continue
			}
			if t.Name.Local == "style" {
				inStyle = true
			}
			b.WriteString(">")
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if t.Name.Local == "style" {
				inStyle = false
			}
			b.WriteString("</" + qname(t.Name) + ">")
			depth--
			if depth == 0 {
				return b.String(), nil
			}
		case xml.CharData:
			if skip > 0 || depth == 0 {
				continue
			}
			if inStyle {
				b.WriteString(strings.ReplaceAll(fixRefs(string(t)), "</", "<\\/"))
			} else {
				b.WriteString(template.HTMLEscapeString(string(t)))
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// dropped
		}
	}
	return "", fmt.Errorf("the <svg> element is never closed")
}

func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}

// svgArgs parses the svg function's arguments: [page] path [options].
func svgArgs(args []any) (*Page, string, svgOptions, error) {
	var p *Page
	if len(args) > 0 {
		if pg, ok := args[0].(*Page); ok {
			p, args = pg, args[1:]
		}
	}
	if len(args) == 0 || len(args) > 2 {
		return nil, "", svgOptions{}, fmt.Errorf(`svg: use {{ svg "path.svg" }} or {{ svg "path.svg" (dict "class" "…" "title" "…") }}`)
	}
	ref, ok := args[0].(string)
	if !ok || ref == "" {
		return nil, "", svgOptions{}, fmt.Errorf("svg: the first argument must be a path")
	}
	var opts svgOptions
	if len(args) == 2 {
		m, ok := args[1].(map[string]any)
		if !ok {
			return nil, "", opts, fmt.Errorf("svg: options must be a dict, e.g. (dict %s)", strconv.Quote("class")+` "logo"`)
		}
		var err error
		if opts, err = parseSVGOptions(m); err != nil {
			return nil, "", opts, err
		}
	}
	return p, ref, opts, nil
}
