"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

import { realtimeConfig } from "@/config/realtime";
import {
  RealtimeChannels,
  isRealtimeDebugEnabled,
  realtimeDebug,
  realtimeManager,
  type RealtimeConnectionState,
  type RealtimeDebugSnapshot,
  type RealtimeEventHandler,
  type RealtimeMessage,
  type RealtimePresenceSnapshot,
  type RealtimePublicationHandler,
} from "@/lib/realtime";
import { useRealtimeContext } from "@/providers/realtime-provider";

export function useRealtime() {
  return useRealtimeContext();
}

export function useConnectionState(): RealtimeConnectionState {
  const { status } = useRealtimeContext();
  return status;
}

/**
 * Subscribe to a channel for the lifetime of the component.
 * Pass `null` / `undefined` to skip. Ref-counted — safe across multiple mounts.
 */
export function useChannel(
  channel: string | null | undefined,
  onPublication: RealtimePublicationHandler,
  options?: { enabled?: boolean },
) {
  const enabled = options?.enabled ?? true;
  const handlerRef = useRef(onPublication);

  useEffect(() => {
    handlerRef.current = onPublication;
  }, [onPublication]);

  useEffect(() => {
    if (!enabled || !channel) return;

    let cancelled = false;
    let unsubscribe: (() => void) | undefined;

    const stableHandler: RealtimePublicationHandler = (message, meta) => {
      handlerRef.current(message, meta);
    };

    void realtimeManager.subscribe(channel, stableHandler).then((unsub) => {
      if (cancelled) {
        unsub();
        return;
      }
      unsubscribe = unsub;
    });

    return () => {
      cancelled = true;
      unsubscribe?.();
    };
  }, [channel, enabled]);
}

/** Listen to a domain event type (or `*`) via the shared dispatcher. */
export function useRealtimeEvent(
  type: string | "*",
  handler: RealtimeEventHandler,
  options?: { enabled?: boolean },
) {
  const enabled = options?.enabled ?? true;
  const handlerRef = useRef(handler);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    if (!enabled) return;
    return realtimeManager.onEvent(type, (message, meta) => {
      handlerRef.current(message, meta);
    });
  }, [type, enabled]);
}

/**
 * Live presence for a channel (`dealer:{id}` / `user:{id}`).
 * Subscribes if needed; refreshes on join/leave (ADR-008).
 */
export function usePresence(
  channel: string | null | undefined,
  options?: { enabled?: boolean },
) {
  const enabled =
    (options?.enabled ?? true) && realtimeConfig.presenceEnabled && !!channel;
  const [snapshot, setSnapshot] = useState<RealtimePresenceSnapshot>({
    clients: {},
    stats: null,
  });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(() =>
    Boolean(enabled && realtimeConfig.presenceEnabled),
  );

  const refresh = useCallback(async () => {
    if (!channel || !realtimeConfig.presenceEnabled) {
      setError(
        realtimeConfig.presenceEnabled
          ? null
          : "Presence is not enabled on the realtime server",
      );
      return;
    }
    setLoading(true);
    try {
      const next = await realtimeManager.presence(channel);
      setSnapshot(next);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Presence request failed");
    } finally {
      setLoading(false);
    }
  }, [channel]);

  useEffect(() => {
    if (!enabled || !channel) return;

    let cancelled = false;
    let stop: (() => void) | undefined;

    void realtimeManager
      .watchPresence(channel, (next) => {
        if (cancelled) return;
        setSnapshot(next);
        setError(null);
        setLoading(false);
      })
      .then((unsub) => {
        if (cancelled) {
          unsub();
          return;
        }
        stop = unsub;
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(
          err instanceof Error ? err.message : "Presence subscription failed",
        );
        setLoading(false);
      });

    return () => {
      cancelled = true;
      stop?.();
    };
  }, [channel, enabled]);

  return {
    ...snapshot,
    loading,
    error,
    refresh,
    supported: realtimeConfig.presenceEnabled,
  };
}

/**
 * Default notification stream — toastable events already handled in provider.
 * Use this to observe toastable (or all) events in a feature.
 */
export function useNotificationStream(
  onEvent?: (message: RealtimeMessage, channel: string) => void,
) {
  const { status } = useRealtimeContext();
  const handlerRef = useRef(onEvent);

  useEffect(() => {
    handlerRef.current = onEvent;
  }, [onEvent]);

  useRealtimeEvent("*", (message, meta) => {
    handlerRef.current?.(message, meta.channel);
  });

  return { status };
}

/** Dev-only debug snapshot. Returns a static empty snapshot in production. */
export function useRealtimeDebug(): RealtimeDebugSnapshot {
  return useSyncExternalStore(
    (onStoreChange) => {
      if (!isRealtimeDebugEnabled) return () => undefined;
      return realtimeDebug.subscribe(onStoreChange);
    },
    () => realtimeDebug.getSnapshot(),
    () => realtimeDebug.getSnapshot(),
  );
}

/** Build default channels from AuthMe numeric ids (ADR-007). */
export function useDefaultRealtimeChannels(options: {
  isPlatform: boolean;
  userId?: number | string | null;
  dealerId?: number | string | null;
}) {
  const { isPlatform, userId, dealerId } = options;

  const channels: string[] = [];
  if (isPlatform) {
    channels.push(RealtimeChannels.systemNotifications());
  }
  if (userId != null && userId !== "") {
    channels.push(RealtimeChannels.user(userId));
  }
  if (dealerId != null && dealerId !== "") {
    channels.push(RealtimeChannels.dealer(dealerId));
  }
  return channels;
}
