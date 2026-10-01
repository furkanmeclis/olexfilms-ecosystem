"use client";

import { Centrifuge } from "centrifuge";
import { signIn } from "next-auth/react";
import QRCode from "qrcode";
import { RefreshCw } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, FieldError } from "@/components/ui/field";
import { realtimeConfig } from "@/config/realtime";
import { useLocale } from "@/providers/locale-provider";

/**
 * QR web sign-in (TEC-91): the browser starts a challenge, shows its code as
 * a QR, and waits for the mobile app to approve it. Status changes arrive on
 * the Centrifugo channel `qr:{code}` (anonymous, subscribe-only token from
 * the start response) with GET status polling as the fallback. On approval
 * Auth.js (`qr-login` provider) exchanges code + secret server side.
 */

export type QRLoginStatus =
  | "loading"
  | "pending"
  | "scanned"
  | "approved"
  | "rejected"
  | "consumed"
  | "expired"
  | "error";

type QRStart = {
  code: string;
  secret: string;
  payload: string;
  expires_at: string;
  realtime: {
    enabled: boolean;
    ws_url?: string;
    channel: string;
    token?: string;
  };
};

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code?: string };
};

const POLL_MS = 3000;

async function startChallenge(): Promise<QRStart> {
  const res = await fetch("/api/v1/auth/qr/start", {
    method: "POST",
    headers: { Accept: "application/json" },
    credentials: "same-origin",
  });
  const env = (await res.json()) as Envelope<QRStart>;
  if (!res.ok || !env.data) throw new Error(env.error?.code ?? "qr_start");
  return env.data;
}

/** Polls the status; 410 means expired. Exported for tests. */
export async function fetchQRStatus(code: string): Promise<QRLoginStatus> {
  const res = await fetch(
    `/api/v1/auth/qr/${encodeURIComponent(code)}/status`,
    { headers: { Accept: "application/json" }, credentials: "same-origin" },
  );
  if (res.status === 410) return "expired";
  if (!res.ok) return "error";
  const env = (await res.json()) as Envelope<{ status?: string }>;
  return normalizeStatus(env.data?.status);
}

export function normalizeStatus(raw: unknown): QRLoginStatus {
  switch (raw) {
    case "pending":
    case "scanned":
    case "approved":
    case "rejected":
    case "consumed":
    case "expired":
      return raw;
    default:
      return "error";
  }
}

const FINAL: ReadonlySet<QRLoginStatus> = new Set([
  "rejected",
  "consumed",
  "expired",
  "error",
]);

export function QRLoginPanel({
  onSignedIn,
}: {
  /** Called after Auth.js stored the session; returns false on failure. */
  onSignedIn: () => Promise<boolean>;
}) {
  const { t } = useLocale();
  const [challenge, setChallenge] = useState<QRStart | null>(null);
  const [image, setImage] = useState<string | null>(null);
  const [rawStatus, setStatus] = useState<QRLoginStatus>("loading");
  const [now, setNow] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const completing = useRef(false);

  // Each attempt (mount, "new code") starts one challenge; state is only set
  // in the promise callbacks.
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let alive = true;
    startChallenge()
      .then(async (next) => {
        const url = await QRCode.toDataURL(next.payload, {
          width: 220,
          margin: 1,
        });
        if (!alive) return;
        completing.current = false;
        setChallenge(next);
        setImage(url);
        setNow(Date.now());
        setStatus("pending");
      })
      .catch(() => {
        if (!alive) return;
        setChallenge(null);
        setStatus("error");
      });
    return () => {
      alive = false;
    };
  }, [attempt]);

  const restart = () => {
    setError(null);
    setStatus("loading");
    setImage(null);
    setChallenge(null);
    setAttempt((n) => n + 1);
  };

  // Live status: Centrifugo (server-side subscription of the guest token)
  // plus polling as the fallback.
  useEffect(() => {
    if (!challenge) return;
    let stopped = false;
    const apply = (next: QRLoginStatus) => {
      if (stopped) return;
      setStatus((prev) =>
        FINAL.has(prev) || prev === "approved" ? prev : next,
      );
    };

    let client: Centrifuge | null = null;
    const rt = challenge.realtime;
    if (rt.enabled && rt.token && realtimeConfig.enabled) {
      client = new Centrifuge(rt.ws_url || realtimeConfig.wsUrl, {
        token: rt.token,
      });
      client.on("publication", (ctx) => {
        if (ctx.channel !== rt.channel) return;
        const data = ctx.data as { status?: unknown } | null;
        apply(normalizeStatus(data?.status));
      });
      client.connect();
    }

    const timer = window.setInterval(async () => {
      try {
        apply(await fetchQRStatus(challenge.code));
      } catch {
        // keep the last known status; the next tick retries
      }
    }, POLL_MS);

    return () => {
      stopped = true;
      window.clearInterval(timer);
      client?.disconnect();
    };
  }, [challenge]);

  // Countdown; the expiry itself is derived at render time.
  useEffect(() => {
    if (!challenge) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [challenge]);

  const secondsLeft = challenge
    ? Math.max(
        0,
        Math.round((new Date(challenge.expires_at).getTime() - now) / 1000),
      )
    : 0;
  const status: QRLoginStatus =
    (rawStatus === "pending" || rawStatus === "scanned") &&
    challenge &&
    secondsLeft === 0
      ? "expired"
      : rawStatus;

  // Approved: exchange code + secret through Auth.js.
  useEffect(() => {
    if (status !== "approved" || !challenge || completing.current) return;
    completing.current = true;
    void (async () => {
      const result = await signIn("qr-login", {
        code: challenge.code,
        secret: challenge.secret,
        redirect: false,
      });
      if (!result || result.error) {
        setError(t("auth.qr.sign_in_failed"));
        setStatus("error");
        return;
      }
      const ok = await onSignedIn();
      if (!ok) setStatus("error");
    })();
  }, [status, challenge, onSignedIn, t]);

  const statusText =
    status === "loading"
      ? t("auth.qr.loading")
      : status === "consumed"
        ? t("auth.qr.status.approved")
        : t(`auth.qr.status.${status}`);

  return (
    <div className="space-y-4 text-center" data-testid="qr-login-panel">
      <div className="space-y-1">
        <p className="font-medium">{t("auth.qr.title")}</p>
        <p className="text-muted-foreground text-sm">
          {t("auth.qr.description")}
        </p>
      </div>

      <div className="flex justify-center">
        {image && (status === "pending" || status === "scanned") ? (
          // eslint-disable-next-line @next/next/no-img-element -- data URL
          <img
            src={image}
            alt={t("auth.qr.image_alt")}
            width={220}
            height={220}
            className="rounded-md border bg-white p-2"
          />
        ) : (
          <div className="bg-muted size-[220px] rounded-md" aria-hidden />
        )}
      </div>

      <p className="text-sm" role="status" aria-live="polite">
        {statusText}
      </p>
      {(status === "pending" || status === "scanned") && secondsLeft > 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("auth.qr.expires_in", { seconds: secondsLeft })}
        </p>
      ) : null}

      {error ? (
        <Field data-invalid={true}>
          <FieldError>{error}</FieldError>
        </Field>
      ) : null}

      {FINAL.has(status) && status !== "consumed" ? (
        <Button type="button" variant="outline" onClick={restart}>
          <RefreshCw aria-hidden />
          {t("auth.qr.refresh")}
        </Button>
      ) : null}
    </div>
  );
}
