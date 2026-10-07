import {
  QUOTE_TOKEN_RE,
  publicQuotePath,
  type QuotePdfNotice,
} from "@/features/public-leads/lib/public-quote";
import {
  clientIpFromHeaders,
  fetchUpstreamStream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Anonymous quote PDF (TEC-320): `/teklif/{token}/pdf?lang=` → Go
 * `GET /v1/public/quotes/{token}/pdf/file?locale=`. Go renders the PDF
 * asynchronously: a 202 (queued) is retried once after a short wait, then
 * the visitor goes back to the page with a "being prepared" notice. Each
 * attempt counts against Go's per-IP limit, hence a single retry. A miss
 * or failure also goes back to the page (303, `?pdf=`).
 */
export const dynamic = "force-dynamic";

/** Wait before the one retry of a queued render. */
const RETRY_DELAY_MS = 2500;

type Context = { params: Promise<{ token: string }> };

function backToPage(
  request: Request,
  token: string,
  lang: string | null,
  notice?: QuotePdfNotice,
) {
  const url = new URL(publicQuotePath(token), request.url);
  if (lang) url.searchParams.set("lang", lang);
  if (notice) url.searchParams.set("pdf", notice);
  return new Response(null, {
    status: 303,
    headers: {
      Location: `${url.pathname}${url.search}`,
      "Cache-Control": "no-store",
      "Referrer-Policy": "no-referrer",
    },
  });
}

export async function GET(request: Request, { params }: Context) {
  const { token } = await params;
  const lang = new URL(request.url).searchParams.get("lang");
  if (!QUOTE_TOKEN_RE.test(token)) return backToPage(request, token, lang);

  const query = new URLSearchParams();
  if (lang) query.set("locale", lang);
  const headers = new Headers({ Accept: "application/pdf" });
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const host = forwardedHostFromHeaders(request.headers);
  if (host) headers.set("X-Forwarded-Host", host);
  const qs = query.toString();
  const path = `public/quotes/${encodeURIComponent(token)}/pdf/file${qs ? `?${qs}` : ""}`;

  let res;
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      res = await fetchUpstreamStream(path, { method: "GET", headers });
    } catch {
      return backToPage(request, token, lang, "unavailable");
    }
    if (res.status !== 202) break;
    await res.body?.cancel().catch(() => undefined);
    if (attempt === 0) await new Promise((r) => setTimeout(r, RETRY_DELAY_MS));
  }
  if (!res) return backToPage(request, token, lang, "unavailable");
  if (res.status === 202) return backToPage(request, token, lang, "pending");
  if (res.status === 404) return backToPage(request, token, lang);
  if (res.status === 429)
    return backToPage(request, token, lang, "rate_limited");
  if (res.status !== 200 || !res.body)
    return backToPage(request, token, lang, "unavailable");

  const out = new Headers({
    "Content-Type": "application/pdf",
    "Cache-Control": "no-store",
    "X-Robots-Tag": "noindex, nofollow",
    "X-Content-Type-Options": "nosniff",
    "Referrer-Policy": "no-referrer",
  });
  out.set(
    "Content-Disposition",
    res.headers.get("content-disposition") ??
      `attachment; filename="quote-${token}.pdf"`,
  );
  const length = res.headers.get("content-length");
  if (length) out.set("Content-Length", length);
  return new Response(res.body, { status: 200, headers: out });
}
