package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeConfig is the code: section of muhssg.yaml.
type CodeConfig struct {
	CopyButton     bool     `yaml:"copyButton"`     // add a Copy button to code blocks (default off)
	CopyButtonSkip []string `yaml:"copyButtonSkip"` // <pre> classes that never get one, e.g. [diagram]
}

// copyButtonScript is the whole copy-button feature: it only runs on pages
// that have code blocks (muhssg adds the script to those pages only) and
// leaves the page untouched when JavaScript is off.
const copyButtonScript = `const skip = %s;
const css = ".muhssg-code{position:relative}" +
  ".muhssg-copy{position:absolute;top:.5rem;right:.5rem;z-index:1;padding:.2em .65em;" +
  "font:500 .75rem/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;" +
  "color:var(--muhssg-copy-fg,inherit);background:var(--muhssg-copy-bg,rgba(127,127,127,.18));" +
  "border:1px solid var(--muhssg-copy-border,rgba(127,127,127,.45));border-radius:var(--muhssg-copy-radius,4px);" +
  "cursor:pointer;opacity:0;transition:opacity .15s}" +
  ".muhssg-code:hover .muhssg-copy,.muhssg-copy:focus-visible{opacity:1}" +
  "@media (hover:none){.muhssg-copy{opacity:1}}" +
  "@media (prefers-reduced-motion:reduce){.muhssg-copy{transition:none}}";

const blocks = [...document.querySelectorAll("pre")].filter(
  (pre) => pre.querySelector("code") && !skip.some((c) => pre.classList.contains(c)) && !pre.closest(".muhssg-code"),
);

if (blocks.length) {
  const style = document.createElement("style");
  style.textContent = css;
  document.head.append(style);
}

const codeText = (code) => {
  const copy = code.cloneNode(true);
  copy.querySelectorAll(".ln, .lnt").forEach((n) => n.remove()); // line numbers
  return copy.textContent.replace(/\n$/, "");
};

const flash = (button, label) => {
  button.textContent = label;
  clearTimeout(button.muhssgTimer);
  button.muhssgTimer = setTimeout(() => (button.textContent = "Copy"), 1600);
};

for (const pre of blocks) {
  const wrap = document.createElement("div");
  wrap.className = "muhssg-code";
  pre.replaceWith(wrap);
  wrap.append(pre);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "muhssg-copy";
  button.textContent = "Copy";
  button.setAttribute("aria-label", "Copy code to clipboard");
  button.addEventListener("click", async () => {
    const code = pre.querySelector("code");
    try {
      if (!navigator.clipboard || !window.isSecureContext) throw new Error("no clipboard");
      await navigator.clipboard.writeText(codeText(code));
      flash(button, "Copied");
    } catch {
      const range = document.createRange();
      range.selectNodeContents(code);
      const selection = getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      flash(button, "Selected");
    }
  });
  wrap.append(button);
}
`

var preCode = regexp.MustCompile(`(?is)<pre([^>]*)>\s*<code`)
var classAttr = regexp.MustCompile(`(?i)\bclass\s*=\s*["']([^"']*)["']`)

// hasCodeBlock reports whether html contains a <pre><code> block that is not
// excluded by one of the skip classes.
func hasCodeBlock(html []byte, skip []string) bool {
	for _, m := range preCode.FindAllSubmatch(html, -1) {
		classes := ""
		if c := classAttr.FindSubmatch(m[1]); c != nil {
			classes = " " + string(c[1]) + " "
		}
		skipped := false
		for _, s := range skip {
			if strings.Contains(classes, " "+s+" ") {
				skipped = true
				break
			}
		}
		if !skipped {
			return true
		}
	}
	return false
}

// writeCopyButton writes the copy-button script into the output and returns
// the <script> tag that loads it.
func writeCopyButton(cfg *Config, outDir string) (string, error) {
	skip := cfg.Code.CopyButtonSkip
	if skip == nil {
		skip = []string{}
	}
	list, _ := json.Marshal(skip)
	src := []byte(fmt.Sprintf(copyButtonScript, list))
	name := hashedName("muhssg-copy.js", src)
	dst := filepath.Join(outDir, AssetDir, name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf(`<script type="module" src="%s/%s/%s"></script>`, cfg.BasePath, AssetDir, name), nil
}

// injectBeforeHeadEnd adds tag just before </head> (or </body>, or the end).
func injectBeforeHeadEnd(html []byte, tag string) []byte {
	lower := strings.ToLower(string(html))
	for _, marker := range []string{"</head>", "</body>"} {
		if i := strings.Index(lower, marker); i >= 0 {
			return []byte(string(html[:i]) + tag + "\n" + string(html[i:]))
		}
	}
	return append(html, []byte(tag)...)
}
