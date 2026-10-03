import { PUBLIC_CODE_RE } from "@/features/warranty/lib/public-warranty";
import {
  clientIpFromHeaders,
  fetchUpstreamStream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Anonymous warranty PDF (TEC-248): `/garanti/{code}/pdf?lang=&tz=` →
 * Go `GET /v1/public/warranties/{code}/pdf`. No session, no cookies; the
 * client IP and host are forwarded for Go's per-IP limit and brand (K3).
 * A miss or failure goes back to the page (303), which shows the warranty
 * state and, for 429 / errors, a notice (`?pdf=`).
 */
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ code: string }> };

function backToPage(
  request: Request,
  code: string,
  lang: string | null,
  notice?: "rate_limited" | "unavailable",
) {
  const url = new URL(`/garanti/${encodeURIComponent(code)}`, request.url);
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
  const { code } = await params;
  const search = new URL(request.url).searchParams;
  const lang = search.get("lang");
  if (!PUBLIC_CODE_RE.test(code)) return backToPage(request, code, lang);

  const query = new URLSearchParams();
  if (lang) query.set("lang", lang);
  const tz = search.get("tz");
  if (tz) query.set("tz", tz);
  const headers = new Headers({ Accept: "application/pdf" });
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const host = forwardedHostFromHeaders(request.headers);
  if (host) headers.set("X-Forwarded-Host", host);
  const acceptLanguage = request.headers.get("accept-language");
  if (acceptLanguage) headers.set("Accept-Language", acceptLanguage);

  const qs = query.toString();
  let res;
  try {
    res = await fetchUpstreamStream(
      `public/warranties/${encodeURIComponent(code)}/pdf${qs ? `?${qs}` : ""}`,
      { method: "GET", headers },
    );
  } catch {
    return backToPage(request, code, lang, "unavailable");
  }
  if (res.status === 404) return backToPage(request, code, lang);
  if (res.status === 429)
    return backToPage(request, code, lang, "rate_limited");
  if (res.status !== 200 || !res.body)
    return backToPage(request, code, lang, "unavailable");

  const out = new Headers({
    "Content-Type": "application/pdf",
    "Cache-Control": "no-store",
    "X-Robots-Tag": "noindex, nofollow",
    "X-Content-Type-Options": "nosniff",
    "Referrer-Policy": "no-referrer",
  });
  const disposition = res.headers.get("content-disposition");
  out.set(
    "Content-Disposition",
    disposition ?? `attachment; filename="warranty-${code}.pdf"`,
  );
  const length = res.headers.get("content-length");
  if (length) out.set("Content-Length", length);
  return new Response(res.body, { status: 200, headers: out });
}
