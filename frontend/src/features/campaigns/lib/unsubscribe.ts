/** Token format of the campaign e-mail unsubscribe link (TEC-407). */
export const UNSUBSCRIBE_TOKEN_RE = /^[A-Za-z0-9_-]{20,64}$/;

export type UnsubscribeResult = "done" | "invalid" | "error";

/**
 * Posts the unsubscribe token through the BFF (`public/*` needs no
 * session). 200 → done; 400/404 → invalid link; anything else → error.
 */
export async function unsubscribeCampaignMessages(
  token: string,
  fetchImpl: typeof fetch = fetch,
): Promise<UnsubscribeResult> {
  if (!UNSUBSCRIBE_TOKEN_RE.test(token)) return "invalid";
  try {
    const res = await fetchImpl("/api/v1/public/campaigns/unsubscribe", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({ token }),
    });
    if (res.ok) return "done";
    if (res.status === 400 || res.status === 404) return "invalid";
    return "error";
  } catch {
    return "error";
  }
}
