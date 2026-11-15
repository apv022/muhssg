# The muhssg website

The muhssg website, built with muhssg and deployed to GitHub Pages by
`.github/workflows/pages.yml`. Every page is Markdown:

- `content/index.md`: the landing page. Its sections (install, features,
  folder tree and so on) are fields in the front matter.
- `content/404.md`, `content/docs/_index.md`
- The Guide and Reference pages are generated from the repository's
  `README.md` and `docs/reference.md`, so edit those, not `content/docs/`.

`templates/` holds layout only.

```sh
python3 site/sync.py          # regenerate content/docs/
go run . serve --root site    # http://127.0.0.1:4321/muhssg/
```
