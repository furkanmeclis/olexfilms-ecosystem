import type { UpstreamFetcher } from "@/features/warranty/lib/public-warranty";
import type { components } from "@/generated/api";

import { DEALER_CODE_RE } from "./dealer-showcase";

/**
 * Sitemap source (TEC-251): Go `GET /v1/public/dealers` lists only the code
 * and last change of the brand's active dealers and distributors.
 */
export type PublicDealerCode = components["schemas"]["PublicDealerCode"];

/**
 * Codes of the brand served on `forwardedHost` (Go resolves the brand from
 * it, K3/K20). Any failure is an empty list: the sitemap then carries the
 * landing page only instead of failing.
 */
export async function fetchPublicDealerCodes(
  forwardedHost: string | null,
  fetcher: UpstreamFetcher,
): Promise<PublicDealerCode[]> {
  const headers = new Headers({ Accept: "application/json" });
  if (forwardedHost) headers.set("X-Forwarded-Host", forwardedHost);
  try {
    const res = await fetcher("public/dealers", { method: "GET", headers });
    if (res.status !== 200) return [];
    const envelope = JSON.parse(new TextDecoder().decode(res.body)) as {
      data?: { items?: PublicDealerCode[] };
    };
    const items = envelope.data?.items;
    if (!Array.isArray(items)) return [];
    return items.filter(
      (item) =>
        typeof item?.code === "string" && DEALER_CODE_RE.test(item.code),
    );
  } catch {
    return [];
  }
}
