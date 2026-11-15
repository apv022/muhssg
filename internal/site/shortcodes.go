package site

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Shortcodes are snippets you can drop into Markdown:
//
//	{{< youtube id="dQw4w9WgXcQ" >}}
//	{{< note type="warning" >}}Some **Markdown** inside.{{< /note >}}
//
// Each one is rendered by templates/shortcodes/<name>.html. Shortcodes are
// ignored inside code blocks and inline code, and {{</* name */>}} prints
// the shortcode itself instead of running it.

// Shortcode is what a shortcode template receives as ".".
type Shortcode struct {
	Name   string
	Params map[string]string
	Inner  string // raw text between the opening and closing tag
	Page   *Page
	Site   *Site

	positional []string
	render     func(src string) (template.HTML, error)
}

// Get returns a named parameter, or a positional one when given a number.
func (s *Shortcode) Get(key any) string {
	switch k := key.(type) {
	case int:
		if k >= 0 && k < len(s.positional) {
			return s.positional[k]
		}
		return ""
	case string:
		if v, ok := s.Params[k]; ok {
			return v
		}
		if i, err := strconv.Atoi(k); err == nil {
			return s.Get(i)
		}
	}
	return ""
}

// Has reports whether a named parameter was given.
func (s *Shortcode) Has(key string) bool { _, ok := s.Params[key]; return ok }

// InnerHTML renders the inner text as Markdown, including any shortcodes in it.
func (s *Shortcode) InnerHTML() (template.HTML, error) {
	if strings.TrimSpace(s.Inner) == "" {
		return "", nil
	}
	return s.render(s.Inner)
}

// InnerInline renders the inner text as inline Markdown, without a wrapping
// <p>, for shortcodes used in the middle of a sentence.
func (s *Shortcode) InnerInline() (template.HTML, error) {
	h, err := s.InnerHTML()
	return template.HTML(stripParagraph(string(h))), err
}

// stripParagraph removes a single <p>…</p> around inline content.
func stripParagraph(h string) string {
	h = strings.TrimSpace(h)
	if strings.HasPrefix(h, "<p>") && strings.HasSuffix(h, "</p>") && strings.Count(h, "<p>") == 1 {
		return h[3 : len(h)-4]
	}
	return h
}

type scTag struct {
	start, end int // byte range of the whole tag
	name       string
	closing    bool
	selfClose  bool
	args       string
	line       int
}

const maxShortcodeDepth = 8

// expandShortcodes replaces shortcodes in src with placeholder tokens and
// returns the rendered HTML for each token.
func (c *contentRenderer) expandShortcodes(p *Page, src string, depth int) (string, map[string]string, error) {
	out := map[string]string{}
	if !strings.Contains(src, "{{<") {
		return src, out, nil
	}
	if depth > maxShortcodeDepth {
		return "", nil, fmt.Errorf("shortcodes nested more than %d deep", maxShortcodeDepth)
	}
	skip := codeRanges(src)
	tags, err := findTags(src, skip)
	if err != nil {
		return "", nil, err
	}

	// Pair opening and closing tags with a stack.
	type node struct {
		open  scTag
		close *scTag
	}
	var top []node
	var stack []int // indexes into all
	var all []node
	parent := map[int]int{}
	for _, t := range tags {
		if t.closing {
			found := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if all[stack[i]].open.name == t.name {
					found = i
					break
				}
			}
			if found < 0 {
				return "", nil, fmt.Errorf("line %d: {{< /%s >}} closes a shortcode that was never opened", t.line, t.name)
			}
			tc := t
			all[stack[found]].close = &tc
			stack = stack[:found] // anything opened after it was self-standing
			continue
		}
		all = append(all, node{open: t})
		idx := len(all) - 1
		if len(stack) > 0 {
			parent[idx] = stack[len(stack)-1]
		}
		if !t.selfClose {
			stack = append(stack, idx)
		}
	}
	// Top-level nodes are those whose ranges are not inside another paired node.
	for i, n := range all {
		inside := false
		for j, m := range all {
			if j != i && m.close != nil && n.open.start > m.open.start && n.open.end <= m.close.end {
				inside = true
				break
			}
		}
		if !inside {
			top = append(top, n)
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].open.start < top[j].open.start })

	var b strings.Builder
	pos := 0
	for i, n := range top {
		end := n.open.end
		inner := ""
		if n.close != nil {
			inner = src[n.open.end:n.close.start]
			end = n.close.end
		}
		params, positional, err := parseShortcodeArgs(n.open.args)
		if err != nil {
			return "", nil, fmt.Errorf("line %d: shortcode %q: %w", n.open.line, n.open.name, err)
		}
		sc := &Shortcode{
			Name: n.open.name, Params: params, positional: positional,
			Inner: strings.Trim(inner, "\n"), Page: p, Site: c.site,
			render: func(s string) (template.HTML, error) {
				html, err := c.renderFragment(p, s, depth+1)
				return template.HTML(html), err
			},
		}
		html, err := c.tmpl.renderShortcode(sc)
		if err != nil {
			return "", nil, fmt.Errorf("line %d: %w", n.open.line, err)
		}
		token := fmt.Sprintf("MUHSSGSHORTCODE%dD%dX", depth, i)
		out[token] = html
		b.WriteString(unescapeOutside(src, pos, n.open.start, skip))
		b.WriteString(token)
		pos = end
	}
	b.WriteString(unescapeOutside(src, pos, len(src), skip))
	return b.String(), out, nil
}

var escapedTag = regexp.MustCompile(`\{\{<\s*/\*(.*?)\*/\s*>\}\}`)

// unescapeOutside returns src[from:to] with {{</* x */>}} turned into
// {{< x >}}, except inside code, where shortcodes never run and the escape
// is shown exactly as written.
func unescapeOutside(src string, from, to int, skip [][2]int) string {
	if !strings.Contains(src[from:to], "{{<") {
		return src[from:to]
	}
	var b strings.Builder
	pos := from
	for _, r := range skip {
		lo, hi := max(r[0], from), min(r[1], to)
		if lo >= hi {
			continue
		}
		b.WriteString(unescape(src[pos:lo]))
		b.WriteString(src[lo:hi])
		pos = hi
	}
	b.WriteString(unescape(src[pos:to]))
	return b.String()
}

func unescape(s string) string {
	return escapedTag.ReplaceAllStringFunc(s, func(m string) string {
		return "{{< " + strings.TrimSpace(escapedTag.FindStringSubmatch(m)[1]) + " >}}"
	})
}

// findTags locates every shortcode tag outside the skipped ranges.
func findTags(src string, skip [][2]int) ([]scTag, error) {
	var tags []scTag
	i := 0
	for {
		j := strings.Index(src[i:], "{{<")
		if j < 0 {
			break
		}
		start := i + j
		if inRanges(start, skip) {
			i = start + 3
			continue
		}
		k := strings.Index(src[start:], ">}}")
		if k < 0 {
			return nil, fmt.Errorf("line %d: shortcode is never closed with >}}", lineOf(src, start))
		}
		end := start + k + 3
		body := strings.TrimSpace(src[start+3 : end-3])
		i = end
		if strings.HasPrefix(body, "/*") { // escaped: {{</* name */>}}
			continue
		}
		t := scTag{start: start, end: end, line: lineOf(src, start)}
		if strings.HasPrefix(body, "/") {
			t.closing = true
			body = strings.TrimSpace(body[1:])
		}
		if strings.HasSuffix(body, "/") {
			t.selfClose = true
			body = strings.TrimSpace(strings.TrimSuffix(body, "/"))
		}
		name := body
		if sp := strings.IndexAny(body, " \t\n"); sp >= 0 {
			name, t.args = body[:sp], strings.TrimSpace(body[sp:])
		}
		if !validShortcodeName(name) {
			return nil, fmt.Errorf("line %d: %q is not a valid shortcode name", t.line, name)
		}
		if t.closing && t.args != "" {
			return nil, fmt.Errorf("line %d: closing tag {{< /%s >}} cannot have parameters", t.line, name)
		}
		t.name = name
		tags = append(tags, t)
	}
	return tags, nil
}

func validShortcodeName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// parseShortcodeArgs parses `a="x y" b=2 positional 'quoted'`.
func parseShortcodeArgs(s string) (map[string]string, []string, error) {
	params := map[string]string{}
	var positional []string
	i := 0
	readValue := func() (string, error) {
		if i < len(s) && (s[i] == '"' || s[i] == '\'' || s[i] == '`') {
			q := s[i]
			i++
			var b strings.Builder
			for i < len(s) {
				c := s[i]
				if c == '\\' && q != '`' && i+1 < len(s) {
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				if c == q {
					i++
					return b.String(), nil
				}
				b.WriteByte(c)
				i++
			}
			return "", fmt.Errorf("unterminated quote")
		}
		st := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' {
			i++
		}
		return s[st:i], nil
	}
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
			i++
		}
		if i >= len(s) {
			return params, positional, nil
		}
		// key=value?
		st := i
		for i < len(s) && (s[i] == '_' || s[i] == '-' || (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= '0' && s[i] <= '9')) {
			i++
		}
		if i > st && i < len(s) && s[i] == '=' {
			key := s[st:i]
			i++
			v, err := readValue()
			if err != nil {
				return nil, nil, err
			}
			params[key] = v
			continue
		}
		i = st
		v, err := readValue()
		if err != nil {
			return nil, nil, err
		}
		positional = append(positional, v)
	}
}

// codeRanges returns byte ranges of fenced code blocks and inline code spans.
func codeRanges(src string) [][2]int {
	var ranges [][2]int
	lines := strings.SplitAfter(src, "\n")
	pos := 0
	fence := ""
	fenceStart := 0
	var text strings.Builder
	textStart := 0
	flushText := func() {
		t := text.String()
		for _, r := range inlineCode(t) {
			ranges = append(ranges, [2]int{textStart + r[0], textStart + r[1]})
		}
		text.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if fence == "" {
			if indent < 4 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
				flushText()
				ch := trimmed[0]
				n := 0
				for n < len(trimmed) && trimmed[n] == ch {
					n++
				}
				fence = strings.Repeat(string(ch), n)
				fenceStart = pos
			} else {
				if text.Len() == 0 {
					textStart = pos
				}
				text.WriteString(line)
			}
		} else if indent < 4 && strings.HasPrefix(strings.TrimRight(trimmed, " \t\r\n"), fence) &&
			strings.Trim(strings.TrimRight(trimmed, " \t\r\n"), string(fence[0])) == "" {
			ranges = append(ranges, [2]int{fenceStart, pos + len(line)})
			fence = ""
			textStart = pos + len(line)
		}
		pos += len(line)
	}
	if fence != "" {
		ranges = append(ranges, [2]int{fenceStart, len(src)})
	} else {
		flushText()
	}
	return ranges
}

// inlineCode finds `code` spans (matching backtick runs) in text.
func inlineCode(t string) [][2]int {
	var out [][2]int
	i := 0
	for i < len(t) {
		if t[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(t) && t[i+n] == '`' {
			n++
		}
		run := strings.Repeat("`", n)
		j := i + n
		found := -1
		for j < len(t) {
			k := strings.Index(t[j:], run)
			if k < 0 {
				break
			}
			at := j + k
			m := 0
			for at+m < len(t) && t[at+m] == '`' {
				m++
			}
			if m == n {
				found = at
				break
			}
			j = at + m
		}
		if found < 0 {
			i += n
			continue
		}
		out = append(out, [2]int{i, found + n})
		i = found + n
	}
	return out
}

func inRanges(pos int, ranges [][2]int) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

func lineOf(s string, pos int) int { return strings.Count(s[:pos], "\n") + 1 }

// substituteTokens swaps placeholder tokens for rendered shortcode HTML.
// A token alone in a paragraph replaces the whole <p>.
func substituteTokens(html string, tokens map[string]string) string {
	for token, out := range tokens {
		html = strings.ReplaceAll(html, "<p>"+token+"</p>", out)
		html = strings.ReplaceAll(html, token, out)
	}
	return html
}

// renderShortcode executes templates/shortcodes/<name>.html.
func (t *templates) renderShortcode(sc *Shortcode) (string, error) {
	name := "shortcodes/" + sc.Name + ".html"
	set, ok := t.shortcodes[name]
	if !ok {
		return "", fmt.Errorf("unknown shortcode %q (add templates/%s)", sc.Name, name)
	}
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, sc); err != nil {
		return "", fmt.Errorf("shortcode %q: %w", sc.Name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}
