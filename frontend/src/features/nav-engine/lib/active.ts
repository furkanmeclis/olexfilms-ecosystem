function normalizePath(path: string): string {
  if (!path) return "/";
  const trimmed = path.replace(/\/+$/, "");
  return trimmed || "/";
}

/**
 * Picks the most specific nav href that matches the current pathname.
 * Prevents parent routes (e.g. /finance) from staying active on child pages
 * (e.g. /finance/accounts) when siblings exist in the same group.
 */
export function resolveActiveNavHref(
  pathname: string,
  hrefs: string[],
  homeHref?: string,
): string | null {
  const current = normalizePath(pathname);
  const home = homeHref ? normalizePath(homeHref) : null;
  let best: string | null = null;
  let bestLen = -1;

  for (const href of hrefs) {
    const target = normalizePath(href);
    // The shell home (e.g. /t/{slug}) prefixes every page; it is only active
    // on an exact match, never as a fallback for pages in other groups.
    const prefixMatch = target !== home && current.startsWith(`${target}/`);
    if (current === target || prefixMatch) {
      if (target.length > bestLen) {
        best = href;
        bestLen = target.length;
      }
    }
  }

  return best;
}

export function isNavHrefActive(
  pathname: string,
  href: string,
  homeHref: string,
  peerHrefs?: string[],
): boolean {
  if (peerHrefs?.length) {
    return resolveActiveNavHref(pathname, peerHrefs, homeHref) === href;
  }

  const current = normalizePath(pathname);
  const target = normalizePath(href);
  const home = normalizePath(homeHref);

  if (current === target) return true;
  if (target === home) return false;
  return current.startsWith(`${target}/`);
}

export function formatNavCount(value: number, max = 99): string {
  if (value > max) return `${max}+`;
  return String(value);
}
