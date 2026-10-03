import {
  SHORT_URL_TOKEN_RE,
  SHORT_URL_UNAVAILABLE_PATH,
  isAllowedShortUrlTarget,
  type ShortUrlUnavailableReason,
} from "@/features/short-urls/lib/short-url";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Short link (TEC-249): `/s/{token}` → Go `GET /v1/public/short-urls/{token}`
 * and a 302 to the internal target path. No session, no cookies; the
 * client IP and host are forwarded for Go's per-IP limit and brand (K3).
 * Expired (410), unknown (404) and failed lookups go to the
 * `/link-unavailable` page. Not the storage share link `/share/s/{token}`.
 */
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ token: string }> };

const NO_STORE = {
  "Cache-Control": "no-store",
  "Referrer-Policy": "no-referrer",
  "X-Robots-Tag": "noindex, nofollow",
};

function redirect(location: string) {
  return new Response(null, {
    status: 302,
    headers: { ...NO_STORE, Location: location },
  });
}

function unavailable(reason: ShortUrlUnavailableReason) {
  return redirect(`${SHORT_URL_UNAVAILABLE_PATH}?reason=${reason}`);
}

function targetOf(body: ArrayBuffer): unknown {
  try {
    const parsed = JSON.parse(new TextDecoder().decode(body)) as {
      data?: { target_path?: unknown };
    };
    return parsed.data?.target_path;
  } catch {
    return undefined;
  }
}

export async function GET(request: Request, { params }: Context) {
  const { token } = await params;
  if (!SHORT_URL_TOKEN_RE.test(token)) return unavailable("not_found");

  const headers = new Headers({ Accept: "application/json" });
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const host = forwardedHostFromHeaders(request.headers);
  if (host) headers.set("X-Forwarded-Host", host);

  let res;
  try {
    res = await fetchUpstream(`public/short-urls/${token}`, {
      method: "GET",
      headers,
    });
  } catch {
    return unavailable("error");
  }
  if (res.status === 410) return unavailable("expired");
  if (res.status === 404) return unavailable("not_found");
  if (res.status !== 200) return unavailable("error");

  const target = targetOf(res.body);
  if (!isAllowedShortUrlTarget(target)) return unavailable("error");
  return redirect(target);
}
