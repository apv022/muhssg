// Copy buttons on code blocks. The site works fine without this.
for (const pre of document.querySelectorAll<HTMLPreElement>(".prose pre, .term pre")) {
  if (!navigator.clipboard) break;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "copy-button";
  button.textContent = "copy";
  button.addEventListener("click", async () => {
    const text = (pre.querySelector("code") ?? pre).textContent ?? "";
    const lines = pre.closest(".term")
      ? text.split("\n").filter((l) => l.startsWith("$ ")).map((l) => l.slice(2)).join("\n")
      : text.replace(/\n$/, "");
    try {
      await navigator.clipboard.writeText(lines);
      button.textContent = "copied";
    } catch {
      button.textContent = "failed";
    }
    setTimeout(() => (button.textContent = "copy"), 1500);
  });
  pre.style.position = "relative";
  pre.append(button);
}
