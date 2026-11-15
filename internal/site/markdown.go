package site

import (
	"bytes"
	"strings"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Heading is one entry of a page's table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// Rendered is the result of rendering a Markdown document.
type Rendered struct {
	HTML      string
	Headings  []Heading
	Summary   string // plain text of the first paragraph, or of everything before <!--more-->
	WordCount int
}

const moreMarker = "<!--more-->"

func newMarkdown(cfg *Config) goldmark.Markdown {
	exts := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		extension.DefinitionList,
	}
	if boolOr(cfg.Markdown.Typographer, true) {
		exts = append(exts, extension.Typographer)
	}
	if !cfg.Highlight.Disable {
		exts = append(exts, highlighting.NewHighlighting(
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(true),
				chromahtml.WithLineNumbers(cfg.Highlight.LineNumbers),
			),
		))
	}
	var rendererOpts []goldmark.Option
	if boolOr(cfg.Markdown.Unsafe, true) {
		rendererOpts = append(rendererOpts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}
	opts := append([]goldmark.Option{
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	}, rendererOpts...)
	return goldmark.New(opts...)
}

// RenderMarkdown renders src and collects headings, a summary and a word count.
func RenderMarkdown(md goldmark.Markdown, src []byte) (Rendered, error) {
	ctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))

	var out Rendered
	var all strings.Builder
	firstPara := ""
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			id := ""
			if v, ok := node.AttributeString("id"); ok {
				if b, ok := v.([]byte); ok {
					id = string(b)
				} else if s, ok := v.(string); ok {
					id = s
				}
			}
			out.Headings = append(out.Headings, Heading{Level: node.Level, ID: id, Text: plainText(node, src)})
		case *ast.Paragraph:
			if firstPara == "" {
				firstPara = plainText(node, src)
			}
		case *ast.Text:
			all.Write(node.Segment.Value(src))
			all.WriteByte(' ')
		case *ast.String:
			all.Write(node.Value)
			all.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return out, err
	}
	out.HTML = buf.String()
	out.WordCount = countWords(all.String())

	if i := bytes.Index(src, []byte(moreMarker)); i >= 0 {
		head := md.Parser().Parse(text.NewReader(src[:i]))
		out.Summary = strings.TrimSpace(collapseSpace(plainText(head, src[:i])))
	} else {
		out.Summary = firstPara
	}
	return out, nil
}

// plainText returns the visible text inside n, without markup.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.CodeBlock, *ast.FencedCodeBlock, *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.Heading, *ast.ListItem:
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(collapseSpace(b.String()))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func countWords(s string) int {
	n := 0
	for _, f := range strings.FieldsFunc(s, unicode.IsSpace) {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				n++
				break
			}
		}
	}
	return n
}

// Truncate shortens s to at most max runes, cutting at a word boundary and
// adding an ellipsis when it had to cut.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndexAny(cut, " \t\n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-—") + "…"
}
