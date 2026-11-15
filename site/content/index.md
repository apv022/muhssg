---
title: "muhssg"
description: "A static site generator for personal sites. Markdown in, fast static HTML out. One binary, no Node, no runtime."
---

{{< hero alt="A terminal window with the word muhssg drawn in crimson pixel blocks" lede='A static site generator for personal sites.<br>Markdown in, fast static HTML out. <span class="sun">One binary.</span> No Node, no runtime.' >}}

{{< button url="/docs/guide/" >}}Read the guide →{{< /button >}}

{{< button url="https://github.com/apv022/muhssg" quiet="true" >}}View on GitHub ↗{{< /button >}}

{{< /hero >}}

{{< terminal label="install" result="built 9 pages in 226ms" serving="serving my-site at" url="http://127.0.0.1:4321/" hint="— Ctrl+C to stop" note="Prefer a binary? Grab one for Linux, macOS or Windows from [releases](https://github.com/apv022/muhssg/releases)." >}}
go install github.com/apv022/muhssg@latest
muhssg new site my-site && cd my-site
muhssg serve
{{< /terminal >}}

{{< features label="what you get" >}}

{{< feature title="Static first" >}}Pages are plain HTML that works with JavaScript off. Add TypeScript only where a page needs to move.{{< /feature >}}

{{< feature title="Collections" >}}Every folder in `content/` gets a list page, prev/next links, tags and an RSS feed.{{< /feature >}}

{{< feature title="Assets, bundled" >}}CSS imports and JS/TS are bundled by esbuild, minified and content-hashed.{{< /feature >}}

{{< feature title="Swap the vibe" >}}Themes stack. A theme can be one CSS file that restyles everything below it.{{< /feature >}}

{{< feature title="Shortcodes" >}}Drop `{{< youtube >}}`, figures, notes and your own snippets straight into Markdown.{{< /feature >}}

{{< feature title="Images" >}}Local images become responsive srcsets. Phone photos are rotated and stripped of GPS data.{{< /feature >}}

{{< feature title="Pagination" >}}`paginate: 10` splits long lists into `/page/2/`, `/page/3/` and so on.{{< /feature >}}

{{< feature title="Highlighting" >}}Code is coloured at build time, with separate light and dark styles and no JavaScript.{{< /feature >}}

{{< feature title="Favicons" >}}One square image becomes the .ico, Apple and Android icons and a web manifest.{{< /feature >}}

{{< feature title="SEO, done" >}}Canonical links, Open Graph tags, a sitemap, robots.txt and redirects.{{< /feature >}}

{{< feature title="Live reload" >}}`muhssg serve` rebuilds on save and shows build errors right on the page.{{< /feature >}}

{{< feature title="Safe builds" >}}Output is swapped in only after a build succeeds. Deleted pages disappear.{{< /feature >}}

{{< feature title="Link checker" >}}`muhssg check` catches broken links and missing anchors.{{< /feature >}}

{{< /features >}}

{{< themes label="one line, new look" alt="A blog post in the default light theme beside the same post in the dark solar theme" >}}
The same page, before and after adding a theme that is a single CSS file of colour tokens:
{{< /themes >}}

{{< folders label="a site" root="my-site/" text="No config is required beyond a title. Every folder is optional, and each one does what its name says." link="Read the guide →" url="/docs/guide/" >}}
muhssg.yaml
content/ | Markdown and HTML pages
templates/ | html/template layouts
assets/ | CSS, JS, TS: bundled
static/ | copied as-is
data/ | YAML and JSON
themes/ | layers of all of the above
{{< /folders >}}
