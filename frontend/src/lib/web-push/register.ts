/**
 * Register Web Push via BFF → backend VAPID endpoints.
 * Public key only comes from GET /api/v1/notifications/vapid-public-key (proxied).
 */

export type WebPushSupport = {
  supported: boolean;
  reason?: "browser" | "vapid";
};

const DEFAULT_SW_URL = "/app-push-sw.js";

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

/**
 * Whether this browser exposes the Web Push APIs. iOS Safari outside an
 * installed home-screen app has no `Notification` global at all, so callers
 * must check this before touching `Notification.permission`.
 */
export function hasBrowserPushSupport(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

async function fetchVapidPublicKey(): Promise<string | null> {
  const vapidRes = await fetch("/api/v1/notifications/vapid-public-key", {
    credentials: "include",
  });
  if (!vapidRes.ok) return null;
  const envelope = (await vapidRes.json()) as {
    data?: { vapid_public_key?: string };
  };
  const vapidKey = envelope.data?.vapid_public_key?.trim();
  return vapidKey || null;
}

async function getPushRegistration(serviceWorkerUrl = DEFAULT_SW_URL) {
  const reg = await navigator.serviceWorker.register(serviceWorkerUrl);
  await navigator.serviceWorker.ready;
  return reg;
}

/** Whether this browser can use Web Push and the server has VAPID configured. */
export async function getWebPushSupport(): Promise<WebPushSupport> {
  if (!hasBrowserPushSupport()) {
    return { supported: false, reason: "browser" };
  }
  const vapidKey = await fetchVapidPublicKey();
  if (!vapidKey) {
    return { supported: false, reason: "vapid" };
  }
  return { supported: true };
}

export async function registerWebPush(opts?: {
  serviceWorkerUrl?: string;
  /** When true, only subscribe if permission is already granted (no prompt). */
  silent?: boolean;
}): Promise<PushSubscription | null> {
  if (!hasBrowserPushSupport()) return null;

  const permission = Notification.permission;
  if (permission === "denied") return null;
  if (permission === "default") {
    if (opts?.silent) return null;
    const next = await Notification.requestPermission();
    if (next !== "granted") return null;
  }

  const vapidKey = await fetchVapidPublicKey();
  if (!vapidKey) return null;

  const reg = await getPushRegistration(
    opts?.serviceWorkerUrl ?? DEFAULT_SW_URL,
  );
  const keyBytes = urlBase64ToUint8Array(vapidKey);
  const existing = await reg.pushManager.getSubscription();
  const sub =
    existing ??
    (await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: keyBytes as BufferSource,
    }));

  const json = sub.toJSON();
  const res = await fetch("/api/v1/notifications/push-subscriptions", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      endpoint: json.endpoint,
      key_p256dh: json.keys?.p256dh ?? "",
      key_auth: json.keys?.auth ?? "",
    }),
  });
  if (!res.ok) return null;

  return sub;
}

export async function unregisterWebPush(opts?: {
  serviceWorkerUrl?: string;
}): Promise<void> {
  if (!hasBrowserPushSupport()) return;

  try {
    const reg = await getPushRegistration(
      opts?.serviceWorkerUrl ?? DEFAULT_SW_URL,
    );
    const sub = await reg.pushManager.getSubscription();
    if (!sub) return;

    const endpoint = sub.endpoint;
    await fetch("/api/v1/notifications/push-subscriptions", {
      method: "DELETE",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ endpoint }),
    });
    await sub.unsubscribe();
  } catch {
    // Best-effort cleanup when SW or subscription is missing.
  }
}
