"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { Minus, Radio, X } from "lucide-react";

import { isRealtimeDebugEnabled, realtimeDebug } from "@/lib/realtime";
import type {
  RealtimeConnectionState,
  RealtimeDebugSnapshot,
} from "@/lib/realtime";
import { cn } from "@/lib/utils";

const STORAGE_KEY = "app:realtime-debug-panel";
const MODE_EVENT = "app:realtime-debug-panel-mode";

type PanelMode = "open" | "collapsed" | "hidden";

const SERVER_SNAPSHOT: RealtimeDebugSnapshot = {
  state: "idle",
  wsUrl: null,
  channels: [],
  lastEvent: null,
  lastEventAt: null,
  reconnectCount: 0,
  online: true,
  userId: null,
  numericUserId: null,
  tokenExpiresAt: null,
  lastError: null,
  enabled: true,
};

function subscribeNever() {
  return () => undefined;
}

/** True only after client hydration — avoids SSR/client text mismatch. */
function useIsClient() {
  return useSyncExternalStore(
    subscribeNever,
    () => true,
    () => false,
  );
}

function readStoredMode(): PanelMode {
  try {
    const value = sessionStorage.getItem(STORAGE_KEY);
    if (value === "open" || value === "collapsed" || value === "hidden") {
      return value;
    }
  } catch {
    /* private mode / SSR */
  }
  return "collapsed";
}

function writeStoredMode(mode: PanelMode) {
  try {
    sessionStorage.setItem(STORAGE_KEY, mode);
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new Event(MODE_EVENT));
}

function subscribePanelMode(onStoreChange: () => void) {
  window.addEventListener(MODE_EVENT, onStoreChange);
  return () => window.removeEventListener(MODE_EVENT, onStoreChange);
}

function usePanelMode(): PanelMode {
  return useSyncExternalStore(
    subscribePanelMode,
    readStoredMode,
    () => "collapsed" as const,
  );
}

function statusTone(state: RealtimeConnectionState): {
  dot: string;
  label: string;
} {
  switch (state) {
    case "connected":
      return { dot: "bg-emerald-500", label: "Connected" };
    case "connecting":
    case "reconnecting":
      return { dot: "bg-amber-500 animate-pulse", label: "Connecting" };
    case "error":
    case "offline":
      return { dot: "bg-red-500", label: "Error" };
    case "disconnected":
      return { dot: "bg-orange-500", label: "Disconnected" };
    default:
      return { dot: "bg-zinc-400", label: "Idle" };
  }
}

/**
 * Floating debug HUD — Next.js-style collapsible indicator.
 * Client-only after hydration; only when NODE_ENV === "development".
 */
export function RealtimeDebugPanel() {
  const isClient = useIsClient();
  const mode = usePanelMode();

  const snapshot = useSyncExternalStore(
    (onStoreChange) => {
      if (!isRealtimeDebugEnabled) return () => undefined;
      return realtimeDebug.subscribe(onStoreChange);
    },
    () => realtimeDebug.getSnapshot(),
    () => SERVER_SNAPSHOT,
  );

  const setPanelMode = useCallback((next: PanelMode) => {
    writeStoredMode(next);
  }, []);

  useEffect(() => {
    if (!isClient || !isRealtimeDebugEnabled) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(
        event.altKey &&
        event.shiftKey &&
        event.key.toLowerCase() === "r"
      )) {
        return;
      }
      event.preventDefault();
      const current = readStoredMode();
      writeStoredMode(current === "hidden" ? "collapsed" : "hidden");
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [isClient]);

  if (!isRealtimeDebugEnabled || !isClient || mode === "hidden") {
    return null;
  }

  const tone = statusTone(snapshot.state);
  const userId = snapshot.userId ?? snapshot.numericUserId;

  if (mode === "collapsed") {
    return (
      <button
        type="button"
        aria-label={`Realtime debug — ${tone.label}. Open panel`}
        title={`Realtime · ${snapshot.state}`}
        onClick={() => setPanelMode("open")}
        className={cn(
          "border-border bg-background/95 text-foreground",
          "fixed start-3 bottom-3 z-[100] flex items-center gap-2 rounded-full",
          "border px-2.5 py-1.5 font-mono text-[11px] shadow-md",
          "hover:bg-accent transition-colors",
        )}
      >
        <span
          className={cn("size-2 shrink-0 rounded-full", tone.dot)}
          aria-hidden
        />
        <Radio
          className="text-muted-foreground size-3.5 shrink-0"
          aria-hidden
        />
        <span className="font-medium tracking-wide">RT</span>
        <span className="text-muted-foreground max-w-[7rem] truncate">
          {snapshot.state}
        </span>
      </button>
    );
  }

  return (
    <aside
      aria-label="Realtime debug"
      className={cn(
        "border-border bg-background/95",
        "fixed start-3 bottom-3 z-[100] w-[min(100vw-1.5rem,22rem)]",
        "overflow-hidden rounded-lg border shadow-lg",
      )}
    >
      <header className="border-border flex items-center gap-2 border-b px-2.5 py-1.5">
        <span
          className={cn("size-2 shrink-0 rounded-full", tone.dot)}
          aria-hidden
        />
        <span className="text-foreground flex-1 font-mono text-[11px] font-semibold tracking-wide">
          Realtime
        </span>
        <button
          type="button"
          aria-label="Minimize realtime debug"
          title="Minimize"
          onClick={() => setPanelMode("collapsed")}
          className="text-muted-foreground hover:bg-accent hover:text-foreground rounded p-1 transition-colors"
        >
          <Minus className="size-3.5" />
        </button>
        <button
          type="button"
          aria-label="Hide realtime debug"
          title="Hide for this session"
          onClick={() => setPanelMode("hidden")}
          className="text-muted-foreground hover:bg-accent hover:text-foreground rounded p-1 transition-colors"
        >
          <X className="size-3.5" />
        </button>
      </header>

      <dl className="text-muted-foreground grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 p-3 font-mono text-[11px] leading-relaxed">
        <dt>state</dt>
        <dd className="text-foreground">{snapshot.state}</dd>
        <dt>enabled</dt>
        <dd className="text-foreground">
          {snapshot.enabled ? "true" : "false"}
        </dd>
        <dt>online</dt>
        <dd className="text-foreground">
          {snapshot.online ? "true" : "false"}
        </dd>
        <dt>reconnects</dt>
        <dd className="text-foreground">{snapshot.reconnectCount}</dd>
        <dt>user</dt>
        <dd className="text-foreground truncate" title={userId ?? undefined}>
          {userId ?? "—"}
        </dd>
        <dt>token</dt>
        <dd
          className="text-foreground truncate"
          title={snapshot.tokenExpiresAt ?? undefined}
        >
          {snapshot.tokenExpiresAt ?? "—"}
        </dd>
        <dt>channels</dt>
        <dd className="text-foreground break-all">
          {snapshot.channels.length > 0 ? snapshot.channels.join(", ") : "—"}
        </dd>
        <dt>last</dt>
        <dd className="text-foreground break-all">
          {snapshot.lastEvent?.type ?? "—"}
        </dd>
        <dt>ws</dt>
        <dd
          className="text-foreground truncate"
          title={snapshot.wsUrl ?? undefined}
        >
          {snapshot.wsUrl ?? "—"}
        </dd>
        <dt>error</dt>
        <dd
          className={cn(
            "break-all",
            snapshot.lastError ? "text-red-500" : "text-foreground",
          )}
        >
          {snapshot.lastError ?? "—"}
        </dd>
      </dl>
    </aside>
  );
}
