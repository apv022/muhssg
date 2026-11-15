# The muhssg website

The muhssg website, built with muhssg and deployed to GitHub Pages by
`.github/workflows/pages.yml`. The docs pages are generated from the
repository's `README.md` and `docs/reference.md`, so edit those, not
`content/docs/`.

```sh
python3 site/sync.py          # regenerate content/docs/
go run . serve --root site    # http://127.0.0.1:4321/muhssg/
```
