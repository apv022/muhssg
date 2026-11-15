#!/usr/bin/env python3
"""Generate the website's docs pages from README.md and docs/reference.md,
so the site and the repository never disagree. Run from anywhere:

    python3 site/sync.py
"""
import re
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SITE = ROOT / "site"
DOCS = SITE / "content" / "docs"
REPO = "https://github.com/apv022/muhssg"


def links(text: str) -> str:
    text = re.sub(r"\(docs/reference\.md(#[^)]*)?\)", lambda m: f"(/docs/reference/{m.group(1) or ''})", text)
    text = text.replace("(../README.md)", "(/docs/guide/)")
    text = text.replace("(LICENSE)", f"({REPO}/blob/main/LICENSE)")
    text = text.replace("(go.mod)", f"({REPO}/blob/main/go.mod)")
    text = text.replace('src=".github/assets/themes.png"', 'src="/img/themes.png"')
    return text


def page(name: str, title: str, description: str, weight: int, body: str) -> None:
    front = f"---\ntitle: {title}\ndescription: {description}\nweight: {weight}\ntoc: true\n---\n\n"
    (DOCS / f"{name}.md").write_text(front + links(body).strip() + "\n", encoding="utf-8")


def main() -> None:
    DOCS.mkdir(parents=True, exist_ok=True)
    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    start = readme.index("## A site, folder by folder")
    end = readme.index("## License")
    page("guide", "Guide", "Folders, content, templates, assets, themes and deploying.", 1, readme[start:end])

    ref = (ROOT / "docs" / "reference.md").read_text(encoding="utf-8")
    ref = ref.split("\n", 1)[1]  # drop the "# muhssg reference" heading
    ref = re.sub(r"^Everything muhssg understands.*?\n\n", "", ref.lstrip(), flags=re.S)
    page("reference", "Reference", "Every config key, front matter field, template function and command.", 2, ref)

    img = SITE / "static" / "img"
    img.mkdir(parents=True, exist_ok=True)
    shutil.copy(ROOT / ".github" / "assets" / "themes.png", img / "themes.png")
    shutil.copy(ROOT / ".github" / "assets" / "logo.svg", img / "logo.svg")
    print("synced docs into", DOCS.relative_to(ROOT))


if __name__ == "__main__":
    main()
