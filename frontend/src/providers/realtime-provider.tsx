"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { realtimeConfig } from "@/config/realtime";
import {
  RealtimeChannels,
  RealtimeEvents,
  ToastableRealtimeEvents,
  isRealtimeDebugEnabled,
  realtimeDebug,
  realtimeManager,
  type RealtimeConnectionState,
  type RealtimeMessage,
  type RealtimePublicationHandler,
} from "@/lib/realtime";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { RealtimeDebugPanel } from "@/components/realtime/realtime-debug-panel";

type RealtimeContextValue = {
  status: RealtimeConnectionState;
  online: boolean;
  reconnectCount: number;
  connect: () => Promise<void>;
  disconnect: () => void;
  subscribe: (
    channel: string,
    onPublication: RealtimePublicationHandler,
  ) => Promise<() => void>;
  unsubscribe: (channel: string) => void;
  onEvent: typeof realtimeManager.onEvent;
};

const RealtimeContext = createContext<RealtimeContextValue | null>(null);

/** Maps backend event `type` → i18n key under `realtime.*`. */
const TOAST_LABEL_KEYS: Record<string, string> = {
  [RealtimeEvents.NotificationFailed]: "realtime.event_notifications_failed",
  [RealtimeEvents.JobCompleted]: "realtime.event_jobs_completed",
  [RealtimeEvents.JobFailed]: "realtime.event_jobs_failed",
  [RealtimeEvents.ImportCompleted]: "realtime.event_imports_completed",
  [RealtimeEvents.ImportFailed]: "realtime.event_imports_failed",
  [RealtimeEvents.ExportCompleted]: "realtime.event_exports_completed",
  [RealtimeEvents.ExportFailed]: "realtime.event_exports_failed",
  [RealtimeEvents.ExportCancelled]: "realtime.event_exports_cancelled",
  [RealtimeEvents.ReportExportCompleted]:
    "realtime.event_reports_export_completed",
  [RealtimeEvents.ReportExportFailed]: "realtime.event_reports_export_failed",
};

function isPlatformUser(roles: string[]) {
  return roles.some((role) =>
    (realtimeConfig.platformRoles as readonly string[]).includes(role),
  );
}

function toastLabelForEvent(
  message: RealtimeMessage,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  const key = TOAST_LABEL_KEYS[message.type];
  if (key) return t(key);

  const description =
    typeof message.data.description === "string" &&
    message.data.description.length > 0 &&
    message.data.description !== message.type
      ? message.data.description
      : null;
  if (description) return description;

  return t("realtime.event_fallback", { type: message.type });
}

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, bootstrapped, user } = useAuth();
  const { t } = useLocale();
  const [status, setStatus] = useState(realtimeManager.getStatus());
  const [online, setOnline] = useState(true);
  const [reconnectCount, setReconnectCount] = useState(0);
  const defaultUnsubs = useRef<Array<() => void>>([]);
  const wasConnected = useRef(false);
  const tRef = useRef(t);

  useEffect(() => {
    tRef.current = t;
  }, [t]);

  useEffect(() => {
    const offStatus = realtimeManager.subscribeStatus(setStatus);
    const offOnline = realtimeManager.subscribeOnline(setOnline);
    const offReconnect = realtimeManager.subscribeReconnect(setReconnectCount);
    return () => {
      offStatus();
      offOnline();
      offReconnect();
    };
  }, []);

  // Toast stream for curated backend events + reconnect notices
  useEffect(() => {
    return realtimeManager.onEvent("*", (message) => {
      if (!ToastableRealtimeEvents.has(message.type)) return;
      const label = toastLabelForEvent(message, tRef.current);
      if (
        message.type.endsWith(".failed") ||
        message.type.endsWith("_failed")
      ) {
        appToast.error(label);
        return;
      }
      appToast.info(label);
    });
  }, []);

  useEffect(() => {
    if (status === "connected") {
      if (wasConnected.current === false && reconnectCount > 0) {
        appToast.success(t("realtime.reconnected"));
      }
      wasConnected.current = true;
    }
  }, [status, reconnectCount, t]);

  useEffect(() => {
    if (!online && wasConnected.current) {
      appToast.warning(t("realtime.offline"));
    }
  }, [online, t]);

  const clearDefaultChannels = useCallback(() => {
    defaultUnsubs.current.forEach((unsub) => unsub());
    defaultUnsubs.current = [];
  }, []);

  useEffect(() => {
    if (!bootstrapped) return;

    realtimeDebug.patch({ enabled: realtimeConfig.enabled });

    if (!realtimeConfig.enabled || !isAuthenticated) {
      clearDefaultChannels();
      realtimeManager.disconnect();
      wasConnected.current = false;
      if (!realtimeConfig.enabled) {
        realtimeDebug.patch({
          state: "idle",
          lastError:
            "NEXT_PUBLIC_REALTIME_ENABLED=false — connect skipped (idle expected)",
        });
      } else if (!isAuthenticated) {
        realtimeDebug.patch({
          lastError: null,
        });
      }
      return;
    }

    let cancelled = false;

    (async () => {
      try {
        await realtimeManager.connect();
        if (cancelled) return;

        const noop: RealtimePublicationHandler = () => undefined;
        const channels: string[] = [];

        if (user && isPlatformUser(user.roles)) {
          channels.push(RealtimeChannels.systemNotifications());
        }

        const userChannel =
          user?.realtimeUserChannel ||
          (user?.uuid ? RealtimeChannels.user(user.uuid) : null);
        if (userChannel) {
          channels.push(userChannel);
        }

        clearDefaultChannels();
        for (const channel of channels) {
          try {
            const unsub = await realtimeManager.subscribe(channel, noop);
            if (cancelled) {
              unsub();
              continue;
            }
            defaultUnsubs.current.push(unsub);
          } catch {
            // Channel may be forbidden for this principal — skip silently.
          }
        }
      } catch (err) {
        // Connection failure / REALTIME_DISABLED (503) — state already error
        // in ConnectionManager; keep lastError visible on the debug panel.
        const message =
          err instanceof Error ? err.message : "Realtime connection failed";
        realtimeDebug.patch({
          state: "error",
          lastError: message,
        });
      }
    })();

    return () => {
      cancelled = true;
      clearDefaultChannels();
    };
  }, [bootstrapped, isAuthenticated, user, clearDefaultChannels]);

  const value = useMemo<RealtimeContextValue>(
    () => ({
      status,
      online,
      reconnectCount,
      connect: () => realtimeManager.connect(),
      disconnect: () => realtimeManager.disconnect(),
      subscribe: (channel, onPublication) =>
        realtimeManager.subscribe(channel, onPublication),
      unsubscribe: (channel) => realtimeManager.unsubscribe(channel),
      onEvent: (type, handler) => realtimeManager.onEvent(type, handler),
    }),
    [status, online, reconnectCount],
  );

  return (
    <RealtimeContext.Provider value={value}>
      {children}
      {isRealtimeDebugEnabled ? <RealtimeDebugPanel /> : null}
    </RealtimeContext.Provider>
  );
}

export function useRealtimeContext() {
  const ctx = useContext(RealtimeContext);
  if (!ctx) {
    throw new Error("useRealtimeContext must be used within RealtimeProvider");
  }
  return ctx;
}
