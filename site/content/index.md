---
title: "muhssg"
description: "A static site generator for personal sites. Markdown in, fast static HTML out. One binary, no Node, no runtime."

hero:
  logo_alt: "A terminal window with the word muhssg drawn in crimson pixel blocks"
  # Inline Markdown; raw HTML is allowed (<br> breaks the line, class="sun" is yellow).
  lede: 'A static site generator for personal sites.<br>Markdown in, fast static HTML out. <span class="sun">One binary.</span> No Node, no runtime.'
  buttons:
    - label: "Read the guide →"
      url: "/docs/guide/"
    - label: "View on GitHub ↗"
      url: "https://github.com/apv022/muhssg"
      quiet: true

install:
  label: "install"
  prompt: "~"
  commands:
    - "go install github.com/apv022/muhssg@latest"
    - "muhssg new site my-site && cd my-site"
    - "muhssg serve"
  result: "built 9 pages in 226ms"
  serving: "serving my-site at"
  url: "http://127.0.0.1:4321/"
  hint: "— Ctrl+C to stop"
  note: "Prefer a binary? Grab one for Linux, macOS or Windows from [releases](https://github.com/apv022/muhssg/releases)."

features:
  label: "what you get"
  # text is inline Markdown
  items:
    - title: "Static first"
      text: "Pages are plain HTML that works with JavaScript off. Add TypeScript only where a page needs to move."
    - title: "Collections"
      text: "Every folder in `content/` gets a list page, prev/next links, tags and an RSS feed."
    - title: "Assets, bundled"
      text: "CSS imports and JS/TS are bundled by esbuild, minified and content-hashed."
    - title: "Swap the vibe"
      text: "Themes stack. A theme can be one CSS file that restyles everything below it."
    - title: "Shortcodes"
      text: "Drop `{{< youtube >}}`, figures, notes and your own snippets straight into Markdown."
    - title: "Images"
      text: "Local images become responsive srcsets. Phone photos are rotated and stripped of GPS data."
    - title: "Pagination"
      text: "`paginate: 10` splits long lists into `/page/2/`, `/page/3/` and so on."
    - title: "Highlighting"
      text: "Code is coloured at build time, with separate light and dark styles and no JavaScript."
    - title: "Favicons"
      text: "One square image becomes the .ico, Apple and Android icons and a web manifest."
    - title: "SEO, done"
      text: "Canonical links, Open Graph tags, a sitemap, robots.txt and redirects."
    - title: "Live reload"
      text: "`muhssg serve` rebuilds on save and shows build errors right on the page."
    - title: "Safe builds"
      text: "Output is swapped in only after a build succeeds. Deleted pages disappear."
    - title: "Link checker"
      text: "`muhssg check` catches broken links and missing anchors."

themes:
  label: "one line, new look"
  text: "The same page, before and after adding a theme that is a single CSS file of colour tokens:"
  image_alt: "A blog post in the default light theme beside the same post in the dark solar theme"

folders:
  label: "a site"
  root: "my-site/"
  tree:
    - name: "muhssg.yaml"
    - name: "content/"
      note: "Markdown and HTML pages"
    - name: "templates/"
      note: "html/template layouts"
    - name: "assets/"
      note: "CSS, JS, TS: bundled"
    - name: "static/"
      note: "copied as-is"
    - name: "data/"
      note: "YAML and JSON"
    - name: "themes/"
      note: "layers of all of the above"
  text: "No config is required beyond a title. Every folder is optional, and each one does what its name says."
  link:
    label: "Read the guide →"
    url: "/docs/guide/"
---
