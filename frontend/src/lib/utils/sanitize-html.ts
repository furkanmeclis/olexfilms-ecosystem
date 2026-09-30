/**
 * Allowlist sanitizer for tenant-authored rich text (contract templates).
 *
 * The HTML comes from an API field any template author can set directly, so
 * it must never reach `dangerouslySetInnerHTML` unfiltered. Parsing happens
 * in an inert DOMParser document (no scripts run, no resources load); only
 * formatting elements and a small attribute set survive.
 */

const ALLOWED_TAGS = new Set([
  "a",
  "abbr",
  "b",
  "blockquote",
  "br",
  "caption",
  "code",
  "col",
  "colgroup",
  "div",
  "em",
  "figcaption",
  "figure",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
  "i",
  "img",
  "li",
  "mark",
  "ol",
  "p",
  "pre",
  "s",
  "small",
  "span",
  "strong",
  "sub",
  "sup",
  "table",
  "tbody",
  "td",
  "tfoot",
  "th",
  "thead",
  "tr",
  "u",
  "ul",
]);

/** Elements removed together with their content. */
const DROP_WITH_CONTENT = new Set([
  "script",
  "style",
  "iframe",
  "frame",
  "frameset",
  "object",
  "embed",
  "applet",
  "template",
  "noscript",
  "svg",
  "math",
  "form",
  "input",
  "button",
  "textarea",
  "select",
  "link",
  "meta",
  "base",
  "title",
]);

const ALLOWED_ATTRS = new Set([
  "align",
  "alt",
  "class",
  "colspan",
  "dir",
  "height",
  "href",
  "rowspan",
  "src",
  "style",
  "target",
  "title",
  "width",
]);

function isSafeUrl(value: string, forImage: boolean): boolean {
  // Strip whitespace/control chars browsers ignore inside schemes.
  const v = value.replace(/[\u0000- ]+/g, "").toLowerCase();
  if (forImage) {
    return (
      v.startsWith("https:") ||
      v.startsWith("/") ||
      /^data:image\/(png|jpe?g|gif|webp);/.test(v)
    );
  }
  return (
    v.startsWith("https:") ||
    v.startsWith("http:") ||
    v.startsWith("mailto:") ||
    v.startsWith("tel:") ||
    v.startsWith("#") ||
    (v.startsWith("/") && !v.startsWith("//"))
  );
}

function cleanElement(el: Element) {
  for (const attr of Array.from(el.attributes)) {
    const name = attr.name.toLowerCase();
    const tag = el.tagName.toLowerCase();
    if (name.startsWith("data-")) continue;
    if (!ALLOWED_ATTRS.has(name)) {
      el.removeAttribute(attr.name);
      continue;
    }
    if (name === "href" && !isSafeUrl(attr.value, false)) {
      el.removeAttribute(attr.name);
    } else if (
      name === "src" &&
      (tag !== "img" || !isSafeUrl(attr.value, true))
    ) {
      el.removeAttribute(attr.name);
    } else if (
      name === "style" &&
      /url\s*\(|expression\s*\(|@import/i.test(attr.value)
    ) {
      el.removeAttribute(attr.name);
    }
  }
  if (el.tagName.toLowerCase() === "a" && el.getAttribute("target")) {
    el.setAttribute("rel", "noopener noreferrer");
  }
}

function walk(parent: Node) {
  for (const child of Array.from(parent.childNodes)) {
    if (child.nodeType === Node.COMMENT_NODE) {
      child.remove();
      continue;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue;
    const el = child as Element;
    const tag = el.tagName.toLowerCase();
    if (DROP_WITH_CONTENT.has(tag)) {
      el.remove();
      continue;
    }
    walk(el);
    if (!ALLOWED_TAGS.has(tag)) {
      // Unwrap unknown elements but keep their (already cleaned) children.
      el.replaceWith(...Array.from(el.childNodes));
      continue;
    }
    cleanElement(el);
  }
}

/**
 * Returns sanitized HTML. Outside the browser (no DOMParser) it returns an
 * empty string rather than unsanitized markup.
 */
export function sanitizeRichHtml(html: string | null | undefined): string {
  if (!html) return "";
  if (typeof DOMParser === "undefined") return "";
  const doc = new DOMParser().parseFromString(
    `<!DOCTYPE html><body>${html}</body>`,
    "text/html",
  );
  walk(doc.body);
  return doc.body.innerHTML;
}
