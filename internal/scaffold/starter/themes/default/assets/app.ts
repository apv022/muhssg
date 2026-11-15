// Progressive enhancement: the site works without this file. It adds a
// "copy" button to code blocks. TypeScript is compiled and bundled by muhssg.

function addCopyButtons(root: ParentNode = document): void {
  if (!navigator.clipboard) return;
  for (const pre of root.querySelectorAll<HTMLPreElement>(".prose pre")) {
    if (pre.querySelector(".copy-button")) continue;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "copy-button";
    button.textContent = "copy";
    button.addEventListener("click", async () => {
      const code = pre.querySelector("code")?.innerText ?? pre.innerText;
      try {
        await navigator.clipboard.writeText(code.replace(/\n$/, ""));
        button.textContent = "copied";
      } catch {
        button.textContent = "failed";
      }
      setTimeout(() => (button.textContent = "copy"), 1500);
    });
    pre.append(button);
  }
}

addCopyButtons();
