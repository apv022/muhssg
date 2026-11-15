---
title: Hello, world
description: A tour of what a muhssg post can do.
date: 2026-01-01
tags: [meta, muhssg]
toc: true
---

This post lives at `content/writing/hello-world.md` and is published at
`/writing/hello-world/`. Its first paragraph becomes the summary used in
feeds and link previews, unless the front matter sets `description`.

## Code

Fenced code blocks are highlighted at build time, with no JavaScript:

```go
package main

import "fmt"

func main() {
	fmt.Println("hello from muhssg")
}
```

```ts
const greet = (name: string): string => `hello, ${name}`;
```

## Tables and footnotes

| Feature        | Where it lives       |
| -------------- | -------------------- |
| Pages          | `content/`           |
| Layouts        | `templates/`         |
| CSS, JS and TS | `assets/`            |
| Copied as-is   | `static/`            |

Footnotes work too.[^1]

## Drafts

Set `draft: true` in the front matter to keep a post out of the build. It
still shows up under `muhssg serve` so you can preview it.

[^1]: Like this one.
