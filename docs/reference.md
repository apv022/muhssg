# muhssg reference

Everything muhssg understands, in one place. For a tour, start with the
[README](../README.md).

- [muhssg.yaml](#muhssgyaml)
- [Content files](#content-files)
- [URLs and permalinks](#urls-and-permalinks)
- [Templates](#templates)
- [Page](#page) · [Site](#site) · [Collection](#collection) · [Tag](#tag)
- [Template functions](#template-functions)
- [Shortcodes](#shortcodes)
- [Images](#images)
- [Pagination](#pagination)
- [Assets](#assets)
- [Themes](#themes)
- [Data files](#data-files)
- [Output](#output)
- [Command line](#command-line)
- [GitHub Pages](#github-pages)

## muhssg.yaml

Unknown keys are an error, so a typo like `titel:` fails loudly instead of
being ignored.

```yaml
title: My Site                 # used in titles, feeds and the manifest
baseURL: https://example.com   # absolute; may include a path, e.g. https://you.github.io/blog/
language: en                   # <html lang>, og:locale, RSS <language>
description: …                 # fallback meta description
author: …                      # available as .Site.Author
image: /og.png                 # fallback link-preview image (a path in static/ or a URL)
theme: [solar, default]        # a name (themes/<name>), a path, or a list; first wins
output: dist                   # output folder, relative to the site

collections:
  writing:
    title: Writing             # also settable in content/writing/_index.md
    description: …
    sort: date                 # date | title | weight
    order: desc                # asc | desc (default: desc for date, asc otherwise)
    permalink: /:section/:slug/
    feed: true                 # write an RSS feed
    feedPath: /rss.xml         # default /<collection>/rss.xml
    feedLimit: 20              # default: every entry
    paginate: 10               # entries per list page; overrides the top-level paginate

permalinks:                    # shorthand for collections.<name>.permalink
  notes: /notes/:year/:slug/

redirects:                     # from: to (to may be another site)
  /old/: /new/
  /twitter/: https://example.com/@me

favicon:
  source: favicon.png          # raster image, at least 180×180 (512×512 is best)
  svg: favicon.svg             # optional, served as /favicon.svg
  background: "#0b0b0b"        # fills behind the Apple touch icon
  themeColor: "#0b0b0b"        # <meta name="theme-color"> and the manifest
  name: My Site                # manifest name (default: title)
  shortName: Site

highlight:
  style: github                # light style, or the only style
  darkStyle: github-dark       # optional; each style is scoped to its colour scheme
  lineNumbers: false
  disable: false

markdown:
  unsafe: true                 # allow raw HTML in Markdown (default true: it's your content)
  typographer: true            # “smart quotes”, en–dashes and …

images:
  widths: [480, 800, 1200, 1600]   # srcset widths (never larger than the original)
  quality: 82                  # JPEG quality, 1–100
  sizes: "100vw"               # default sizes attribute
  process: true                # make local Markdown images responsive

paginate: 0                    # default page size for collections and tags; 0 = one page

minify: true                   # minify CSS and JS in production builds
robots: true                   # write robots.txt unless static/ has one

params:                        # anything; available as .Site.Params
  tagline: …
```

File paths in `favicon` are looked up in the site folder first, then in each
theme.

## Content files

`content/` holds `.md`, `.markdown` and `.html` files. Everything else in
`content/` (images, PDFs and so on) is copied to the same path in the output.
Files and folders whose names start with `.` or `_` are skipped, except
`_index.md`.

| File | Becomes |
| --- | --- |
| `content/index.md` | the home page, `/` |
| `content/404.md` | `/404.html` |
| `content/about.md` | a standalone page, `/about/` |
| `content/writing/_index.md` | the list page for the `writing` collection, `/writing/` |
| `content/writing/hello.md` | an entry, `/writing/hello/` |
| `content/writing/trip/index.md` | an entry in its own folder (a bundle), `/writing/trip/` |
| `content/writing/2026/notes.md` | an entry, `/writing/2026/notes/` |

`.html` content files have front matter like Markdown, but their body is
inserted as-is. Use them for pages that are mostly custom markup.

A collection gets a list page when it has an `_index.md`, an entry in
`collections:`, or a `templates/<collection>/list.html`.

### Front matter

| Key | Type | Notes |
| --- | --- | --- |
| `title` | string | Defaults to the file name, title-cased. |
| `description` | string | Meta description, list summaries and feeds. |
| `summary` | string | Overrides the automatic summary. |
| `date` | date | `2026-01-01`, `2026-01-01 14:30`, `2026-01-01T14:30:00`, or RFC 3339 with a zone. |
| `updated` | date | Shown as the last-modified date in the sitemap and Open Graph tags. |
| `draft` | bool | Built only by `muhssg serve` and `muhssg build --drafts`. |
| `tags` | list | Grouped into `/tags/<slug>/` when the theme has `tag.html`. |
| `slug` | string | Replaces the file name in the URL. |
| `permalink` | path | The exact URL, overriding everything else. |
| `aliases` | list | Old URLs that redirect here. |
| `layout` | string | Template name, e.g. `wide` uses `templates/wide.html`. |
| `image` | path/URL | Link-preview image. |
| `weight` | int | For `sort: weight`. |
| `toc` | bool | Sets `.ShowTOC` for the template. |
| `noindex` | bool | Leaves the page out of the sitemap and adds `<meta name="robots" content="noindex">`. |

Any other key goes into `.Params`.

The **summary** is the `summary` key, or else everything before a
`<!--more-->` line, or else the first paragraph, as plain text.

## URLs and permalinks

Page URLs end in `/` and are written as `<path>/index.html`. A permalink that
ends in a file name, like `/feed.xml` or `/404.html`, is written as that
file.

Permalink patterns can use:

| Token | Value |
| --- | --- |
| `:section` | the collection name |
| `:slug` | the slug (file name or `slug:`) |
| `:filepath` | subfolders inside the collection plus the slug |
| `:year`, `:month`, `:day` | from `date` (it's an error if a page has none) |

Two pages with the same URL are an error. So is an alias that collides with a
page.

## Templates

Templates are `html/template` files in `templates/`. Names are paths relative
to that folder, such as `partials/header.html` or `writing/single.html`.

- **`base.html`** is the page shell. A layout that only contains
  `{{ define … }}` blocks fills in `base.html`'s `{{ block … }}`s.
- **A layout with other content** at the top level is used on its own, with no
  `base.html`.
- **Partials** in `partials/` are available to every template:
  `{{ template "partials/x.html" . }}`.

Lookup order:

| Page | Templates tried |
| --- | --- |
| with `layout: x` | `<collection>/x.html`, `x.html` |
| home | `index.html`, `home.html`, `page.html` |
| standalone page | `page.html` |
| collection entry | `<collection>/single.html`, `single.html`, `page.html` |
| collection list | `<collection>/list.html`, `list.html` |
| 404 | `404.html`, `page.html` |
| one tag | `tag.html` |
| all tags | `tags.html` |

A home page and a 404 page are generated from `index.html` and `404.html`
even if there is no `content/index.md` or `content/404.md`.

### Page

`.` in a template is the current page.

| Field | |
| --- | --- |
| `.Kind` | `home`, `page`, `single`, `list`, `404`, `tag`, `tags` |
| `.Title`, `.Description`, `.Summary` | |
| `.Content` | rendered HTML |
| `.URL` | path with the base path, for links |
| `.Permalink` | absolute URL |
| `.Path` | site path without the base path |
| `.Section` | collection name, or empty |
| `.Slug` | |
| `.Date`, `.Updated`, `.LastMod` | `time.Time`; `.HasDate` is false when there's no date |
| `.Draft`, `.NoIndex`, `.ShowTOC` | |
| `.Tags`, `.Aliases`, `.Weight`, `.Layout`, `.Image` | |
| `.Params` | extra front matter |
| `.Headings` | every heading: `.Level`, `.ID`, `.Text` |
| `.TOC` | h2–h4 headings that have an ID |
| `.WordCount`, `.ReadingTime` | reading time in whole minutes, at least 1 |
| `.Pages` | every entry of a list page, or every page of a tag |
| `.Paginator` | list and tag pages: the entries for this page number (see [Pagination](#pagination)) |
| `.Prev`, `.Next` | neighbours in the collection's sort order |
| `.Source` | path of the content file |
| `.Site` | the site |
| `.IsHome`, `.IsList`, `.IsSingle` | |

### Site

| Field | |
| --- | --- |
| `.Site.Title`, `.Description`, `.Language`, `.Author`, `.BaseURL`, `.Image` | from the config |
| `.Site.Params` | `params:` from the config |
| `.Site.Data` | data files |
| `.Site.Pages` | every page that's rendered |
| `.Site.RegularPages` | standalone pages and collection entries, newest first |
| `.Site.Collections` | map of name to collection |
| `.Site.Collection "name"` | one collection, or nil |
| `.Site.Tags` | every tag, sorted by slug |
| `.Site.Home` | the home page |
| `.Site.GetPage "/about/"` | find a page by URL or content file (`"about.md"`) |
| `.Site.AbsURL`, `.Site.RelURL` | like the functions below |
| `.Site.Dev` | true under `muhssg serve` |
| `.Site.BuildTime`, `.Site.Generator` | |

### Collection

`.Name`, `.Title`, `.Description`, `.Pages` (sorted, drafts excluded), `.List`
(the list page, or nil), `.Config`.

### Tag

`.Name`, `.Slug`, `.URL`, `.Pages` (newest first).

## Template functions

Arguments come first and the thing being worked on comes last, so functions
chain with pipes: `{{ .Title | truncate 40 | upper }}`.

**Head and assets**

| Function | |
| --- | --- |
| `seo .` | description, canonical, Open Graph, Twitter, feed `<link>`s, robots and generator tags |
| `fullTitle .` | `"Page — Site"`, or just the site title on the home page |
| `favicons` | `<link>` tags for the favicon set |
| `css "site.css"` | `<link rel="stylesheet">` for a bundled asset |
| `js "app.ts"` | `<script type="module">` for a bundled asset |
| `asset "img/logo.png"` | the hashed URL of any asset |
| `hasAsset "syntax.css"` | whether an asset exists |

`css`, `js` and `asset` stop the build with a clear error if the asset doesn't
exist.

**URLs**

| Function | |
| --- | --- |
| `relURL "/about/"` | adds the base path |
| `absURL "/about/"` | the full URL, including `baseURL` |
| `active . "/writing/"` | whether the current page is under that path (`"/"` matches only the home page) |
| `tagURL "Go"` | `/tags/go/` |
| `urlize "Hello, World"` | `hello-world` |

**Pages**

| Function | |
| --- | --- |
| `collection "writing"` | a collection's pages |
| `page "/about/"` | a page by URL or content file |
| `first 3 pages`, `after 3 pages` | the first N, or everything after the first N |
| `reverse pages` | |
| `sortBy "Title" pages` | `Date`, `Title` or `Weight`; prefix with `-` to reverse, e.g. `"-Date"` |
| `where "Tag" "go" pages` | filter by `Section`, `Kind` or `Tag` |
| `groupByYear pages` | `[]{ .Year, .Pages }`, in the pages' order |

**Images** (see [Images](#images))

| Function | |
| --- | --- |
| `img "path" "alt" ["sizes"] ["class"]` | a responsive `<img>` with `srcset`, `width`/`height` and lazy loading |
| `img .Page "photo.jpg" "alt"` | the same, looking next to that page first (page bundles) |
| `image "path"` | an image with `.URL`, `.Width`, `.Height` and `.Resize 400` |

**Dates**: `date "Jan 2, 2006" .Date` uses
[Go's reference date](https://pkg.go.dev/time#pkg-constants). `isoDate .Date`
gives `2006-01-02`. `now` is the build time. A zero date formats as an empty
string.

**Text**: `markdown` (shortcodes work inside it too), `markdownInline` (no
wrapping `<p>`), `plainify` (strip tags), `truncate N`, `lower`, `upper`,
`trim`, `replace "old" "new"`, `split ","`, `join ", " list`, and
`contains`, `hasPrefix` and `hasSuffix`, which take the pattern first so they
chain: `{{ if .Title | hasPrefix "Draft" }}`.

`errorf "message %s" value` stops the build with an error, which is useful
for checking shortcode parameters.

**Values**: `default "fallback" value`, `dict "k" v …`, `list a b …`,
`jsonify` (safe for `<script>`), `add`, `sub`.

**Trusted content** (bypasses escaping, so use only with content you wrote):
`safeHTML`, `safeURL`, `safeCSS`, `safeJS`.

Go templates also have `len`, `index`, `slice`, `printf`, `eq`, `ne`, `lt`,
`gt`, `and`, `or` and `not` built in.

## Shortcodes

```markdown
{{< name positional key="value" other='single quotes' >}}
{{< name >}}Markdown between the tags.{{< /name >}}
{{< name key="value" />}}
```

- **Templates.** A shortcode is rendered by `templates/shortcodes/<name>.html`,
  looked up in the site, then each theme, then muhssg's built-ins.
- **Pairing.** A tag with a matching closing tag gets the text in between as
  `.Inner`. Paired shortcodes can nest, including inside other paired ones.
- **Code is left alone.** Shortcodes don't run in fenced code blocks or
  inline code. Outside code, `{{</* name */>}}` prints `{{< name >}}`.
- **Placement.** A shortcode alone on its line replaces the whole paragraph,
  so block HTML like `<div>` isn't wrapped in a `<p>`.
- **Errors** name the file and line, e.g. an unknown shortcode or an unclosed
  `{{<`.

In a shortcode template, `.` is the shortcode:

| Field | |
| --- | --- |
| `.Name` | the shortcode's name |
| `.Get "key"` | a named parameter, or `""` |
| `.Get 0` | the first positional parameter, or `""` |
| `.Has "key"` | whether a named parameter was given |
| `.Params` | all named parameters |
| `.Inner` | the raw text between the tags |
| `.InnerHTML` | `.Inner` rendered as Markdown (with shortcodes) |
| `.InnerInline` | the same, without the wrapping `<p>`, for use mid-sentence |
| `.Page`, `.Site` | the page being rendered, and the site |

Built-in shortcodes:

| Shortcode | Parameters |
| --- | --- |
| `youtube` | video id (positional or `id`), `title`, `start` (seconds) |
| `vimeo` | video id (positional or `id`), `title` |
| `figure` | `src`, `alt`, `caption` (Markdown), `sizes`, `class` |
| `img` | `src`, `alt`, `sizes`, `class` |
| `note` | `type` (`note`, `tip`, `warning`, `danger`), `title`; Markdown inside |
| `details` | `summary`, `open="true"`; Markdown inside |

`note` outputs `<aside class="note note-<type>">`, so style `.note` in your
CSS.

## Images

Local `![alt](src)` images in Markdown, the `figure` and `img` shortcodes,
and the `img` and `image` functions all go through the same pipeline:

- **Lookup.** The path is looked up next to the page's content file first
  (page bundles), then in `assets/`, `content/` and `static/` of the site and
  each theme.
- **JPEG and PNG** are decoded, rotated according to their EXIF orientation,
  resized to each of `images.widths` smaller than the original, and
  re-encoded. Re-encoding strips all metadata, including GPS location. The
  original file is not copied to the output.
- **GIF and SVG** are published unchanged.
- **Output** goes to `/_assets/images/<path>-<hash>-<width>.<ext>`.
- **The `src`** defaults to the largest configured width (or the original, if
  it's smaller), and `srcset` lists every variant.
- **Caching.** Resized images are cached in your user cache folder (e.g.
  `~/.cache/muhssg/images`), so rebuilds are fast.
- **Remote images** (`https://…`) are left as they are.
- **Missing local images** in Markdown are left alone, and `muhssg check`
  reports them. In templates and shortcodes, a missing image stops the build.

Set `images.process: false` to leave Markdown images untouched.

## Pagination

With `paginate: N` (top level, or per collection), list and tag pages are
split:

| Page | URL |
| --- | --- |
| entries 1…N | `/writing/` |
| entries N+1…2N | `/writing/page/2/` |
| … | `/writing/page/<n>/` |

`/writing/page/1/` redirects to `/writing/`. Only the first page shows the
`_index.md` content.

`.Paginator` on list and tag pages:

| Field | |
| --- | --- |
| `.Items` | the entries on this page |
| `.PageNumber`, `.TotalPages` | 1-based |
| `.PerPage`, `.TotalItems` | |
| `.HasPrev`, `.HasNext` | |
| `.PrevURL`, `.NextURL` | `""` when there is none |
| `.FirstURL`, `.LastURL` | |
| `.Links` | every page: `.Number`, `.URL`, `.Current` |

## Assets

The site's `assets/` folder is merged over each theme's, so the site wins on
name clashes. Then:

- **Entry points.** Top-level `.css`, `.js`, `.mjs`, `.ts`, `.mts`, `.jsx` and
  `.tsx` files whose names don't start with `_` are bundled by esbuild:
  `@import`s and `import`s are inlined, TypeScript and JSX are compiled, and
  the output targets ES2020 as ES modules with code splitting. Output goes to
  `/_assets/<name>-<HASH>.<ext>`.
- **Files referenced from CSS or JS.** `url(./x.png)` and similar are emitted
  as `/_assets/files/<name>-<HASH>.<ext>`.
- **Root paths are left alone.** `url(/fonts/x.woff2)` and `import "/x.js"`
  point at files in `static/`.
- **Every other file** in `assets/` (images, fonts, …) is copied as
  `/_assets/<path>-<hash>.<ext>` and available through `asset`.
- **A JS entry that imports CSS** also gets a stylesheet: `css "app.css"`
  works for `app.ts`.
- **npm packages.** If a site has a `node_modules/` folder, imports can
  resolve from it. This is optional: muhssg itself never needs Node.
- **Highlighting CSS.** With `highlight:` configured, a `syntax.css` asset is
  generated unless you provide your own.

Production builds are minified (`minify: false` turns that off). `muhssg
serve` skips minification and adds inline source maps.

Because file names change whenever the content does, you can serve
`/_assets/` with a long cache lifetime.

## Themes

`theme:` takes a list of layers. For any file in `templates/`, `assets/`,
`static/` or `data/`, muhssg uses the first match in this order:

1. the site itself
2. the first theme
3. the second theme, and so on

A theme name without a slash means `themes/<name>`. A path is relative to the
site. An absolute path also works, which is useful for a theme shared between
several sites.

## Data files

`data/**/*.yaml`, `*.yml` and `*.json` from the site and every theme become
`.Site.Data`. The path becomes the key: `data/nav/main.yaml` is
`.Site.Data.nav.main`. Site files replace theme files with the same name.

## Output

| Path | |
| --- | --- |
| `/<page>/index.html` | every page |
| `/404.html` | not-found page (marked `noindex`) |
| `/_assets/…` | bundled and hashed assets |
| `/_assets/images/…` | resized images |
| `/<collection>/page/<n>/` | further pages of a paginated list |
| `/<collection>/rss.xml` | feeds (or `feedPath`) |
| `/sitemap.xml` | every page except 404, drafts and `noindex` pages |
| `/robots.txt` | allows everything and points at the sitemap |
| `/favicon.ico`, `/apple-touch-icon.png`, `/icon-192.png`, `/icon-512.png`, `/site.webmanifest` | with `favicon.source` |
| `/<from>/index.html` + `/_redirects` | redirects and aliases |
| everything in `static/` | copied first, so generated files win on clashes |

Each build goes to a temporary folder beside the output and replaces it only
when it has finished. muhssg refuses to build into the site folder itself,
into a source folder, or into a git repository.

## Command line

```
muhssg new site <dir> [--title "Name"]
muhssg new <collection> "Title" [--root dir]
muhssg build [--root dir] [--out dir] [--drafts] [--base-url url]
muhssg serve [--root dir] [--host 127.0.0.1] [--port 4321] [--no-drafts]
muhssg check [--root dir] [--drafts]
muhssg styles
muhssg syntax <style> [--dark <style>]
muhssg version
```

`serve` uses the next free port if 4321 is taken, rebuilds on every change in
the site and its themes, reloads open pages, and shows build errors on top of
the page until they're fixed.

`check` builds into a temporary folder and looks at every `href`, `src` and
`srcset` that points into the site, including `#anchors`. It exits with status
1 if anything is broken.

## GitHub Pages

`.github/workflows/pages.yml` in your site's repository:

```yaml
name: Deploy
on:
  push:
    branches: [main]
permissions:
  contents: read
  pages: write
  id-token: write
concurrency:
  group: pages
  cancel-in-progress: true
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - id: pages
        uses: actions/configure-pages@v6
      - run: go install github.com/apv022/muhssg@latest
      - run: muhssg build --base-url "${{ steps.pages.outputs.base_url }}/"
      - uses: actions/upload-pages-artifact@v5
        with:
          path: dist
      - id: deployment
        uses: actions/deploy-pages@v5
```

`--base-url` makes every link work under `https://<you>.github.io/<repo>/`.
