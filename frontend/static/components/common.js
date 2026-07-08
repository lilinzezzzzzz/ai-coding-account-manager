export function emptyState(title, detail) {
  const section = document.createElement("section");
  section.className = "empty-state";
  const heading = document.createElement("h2");
  heading.textContent = title;
  const text = document.createElement("p");
  text.textContent = detail;
  section.append(heading, text);
  return section;
}
