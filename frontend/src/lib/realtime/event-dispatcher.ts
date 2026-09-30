import type {
  RealtimeEventHandler,
  RealtimeMessage,
} from "@/lib/realtime/types";

type Wildcard = "*";

/**
 * Process-wide event dispatcher. Modules listen by event type
 * (or `*`) without touching Centrifugo.
 */
class RealtimeEventDispatcher {
  private handlers = new Map<string, Set<RealtimeEventHandler>>();

  on(type: string | Wildcard, handler: RealtimeEventHandler): () => void {
    const key = type;
    let set = this.handlers.get(key);
    if (!set) {
      set = new Set();
      this.handlers.set(key, set);
    }
    set.add(handler);
    return () => {
      set!.delete(handler);
      if (set!.size === 0) this.handlers.delete(key);
    };
  }

  emit(message: RealtimeMessage, channel: string) {
    const meta = { channel };
    const specific = this.handlers.get(message.type);
    specific?.forEach((h) => h(message, meta));
    const all = this.handlers.get("*");
    all?.forEach((h) => h(message, meta));
  }

  clear() {
    this.handlers.clear();
  }
}

export const realtimeEventDispatcher = new RealtimeEventDispatcher();
