import type {
  RealtimeDebugSnapshot,
  RealtimeMessage,
} from "@/lib/realtime/types";
import { realtimeConfig } from "@/config/realtime";

const isDev = process.env.NODE_ENV === "development";

type DebugListener = (snapshot: RealtimeDebugSnapshot) => void;

/**
 * Dev-only debug bus. Production builds no-op so no debug surface ships.
 */
class RealtimeDebugStore {
  private snapshot: RealtimeDebugSnapshot = {
    state: "idle",
    wsUrl: null,
    channels: [],
    lastEvent: null,
    lastEventAt: null,
    reconnectCount: 0,
    // Do not read navigator at module init — SSR/client diverge and hydrate badly.
    online: true,
    userId: null,
    numericUserId: null,
    tokenExpiresAt: null,
    lastError: null,
    enabled: realtimeConfig.enabled,
  };
  private listeners = new Set<DebugListener>();

  getSnapshot(): RealtimeDebugSnapshot {
    return this.snapshot;
  }

  subscribe(listener: DebugListener): () => void {
    if (!isDev) return () => undefined;
    this.listeners.add(listener);
    listener(this.snapshot);
    return () => {
      this.listeners.delete(listener);
    };
  }

  patch(partial: Partial<RealtimeDebugSnapshot>) {
    if (!isDev) return;
    // Keep deprecated alias in sync when only one side is patched.
    if (partial.userId !== undefined && partial.numericUserId === undefined) {
      partial = { ...partial, numericUserId: partial.userId };
    } else if (
      partial.numericUserId !== undefined &&
      partial.userId === undefined
    ) {
      partial = { ...partial, userId: partial.numericUserId };
    }
    this.snapshot = { ...this.snapshot, ...partial };
    this.listeners.forEach((l) => l(this.snapshot));
  }

  recordEvent(message: RealtimeMessage) {
    if (!isDev) return;
    this.patch({
      lastEvent: message,
      lastEventAt: new Date().toISOString(),
    });
  }

  incrementReconnect() {
    if (!isDev) return;
    this.patch({ reconnectCount: this.snapshot.reconnectCount + 1 });
  }
}

export const realtimeDebug = new RealtimeDebugStore();
export const isRealtimeDebugEnabled = isDev;
