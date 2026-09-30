import { Centrifuge } from "centrifuge";

import { realtimeConfig } from "@/config/realtime";
import { realtimeDebug } from "@/lib/realtime/debug";
import type {
  ConnectionTokenResult,
  RealtimeConnectionState,
  RealtimeStatusListener,
} from "@/lib/realtime/types";
import { readUserIdFromConnectionToken } from "@/lib/realtime/utils";
import { isApiError } from "@/lib/api";
import { realtimeService } from "@/services/realtime.service";

type OnlineListener = (online: boolean) => void;
type ReconnectListener = (count: number) => void;

function errorMessage(err: unknown): string {
  if (isApiError(err)) {
    return err.message || `HTTP ${err.status}`;
  }
  if (err instanceof Error && err.message) return err.message;
  return "Realtime connection failed";
}

/**
 * Singleton Centrifugo connection — one WebSocket for the entire admin app.
 * No connection pool; pages must not open their own clients.
 */
export class ConnectionManager {
  private client: Centrifuge | null = null;
  private state: RealtimeConnectionState = "idle";
  private statusListeners = new Set<RealtimeStatusListener>();
  private onlineListeners = new Set<OnlineListener>();
  private reconnectListeners = new Set<ReconnectListener>();
  private reconnectCount = 0;
  private intentionalDisconnect = false;
  private connectPromise: Promise<void> | null = null;
  private wsUrl: string | null = null;
  private tokenExpiresAt: string | null = null;
  private userId: string | null = null;
  private online = true;
  private networkBound = false;

  getClient() {
    return this.client;
  }

  getState() {
    return this.state;
  }

  getReconnectCount() {
    return this.reconnectCount;
  }

  getWsUrl() {
    return this.wsUrl;
  }

  getUserId() {
    return this.userId;
  }

  /** @deprecated Prefer `getUserId`. */
  getNumericUserId() {
    return this.userId;
  }

  getTokenExpiresAt() {
    return this.tokenExpiresAt;
  }

  isOnline() {
    return this.online;
  }

  subscribeStatus(listener: RealtimeStatusListener) {
    this.statusListeners.add(listener);
    listener(this.state);
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  subscribeOnline(listener: OnlineListener) {
    this.ensureNetworkListeners();
    this.onlineListeners.add(listener);
    listener(this.online);
    return () => {
      this.onlineListeners.delete(listener);
    };
  }

  subscribeReconnect(listener: ReconnectListener) {
    this.reconnectListeners.add(listener);
    return () => {
      this.reconnectListeners.delete(listener);
    };
  }

  private setState(state: RealtimeConnectionState) {
    this.state = state;
    this.statusListeners.forEach((l) => l(state));
    realtimeDebug.patch({ state });
  }

  private ensureNetworkListeners() {
    if (this.networkBound || typeof window === "undefined") return;
    this.networkBound = true;

    this.online = navigator.onLine;
    this.onlineListeners.forEach((l) => l(this.online));
    realtimeDebug.patch({ online: this.online });

    window.addEventListener("online", () => {
      this.online = true;
      this.onlineListeners.forEach((l) => l(true));
      realtimeDebug.patch({ online: true });
      if (!this.intentionalDisconnect && this.client) {
        this.client.connect();
      }
    });

    window.addEventListener("offline", () => {
      this.online = false;
      this.onlineListeners.forEach((l) => l(false));
      realtimeDebug.patch({ online: false });
      this.setState("offline");
    });
  }

  private async fetchConnectionToken(): Promise<ConnectionTokenResult> {
    const data = await realtimeService.connectionToken();
    this.tokenExpiresAt = data.expires_at;
    this.userId = readUserIdFromConnectionToken(data.token);
    this.wsUrl = data.ws_url || realtimeConfig.wsUrl;
    realtimeDebug.patch({
      tokenExpiresAt: this.tokenExpiresAt,
      userId: this.userId,
      numericUserId: this.userId,
      wsUrl: this.wsUrl,
      lastError: null,
    });
    return data;
  }

  async connect(): Promise<void> {
    realtimeDebug.patch({ enabled: realtimeConfig.enabled });

    if (!realtimeConfig.enabled) {
      this.setState("idle");
      realtimeDebug.patch({
        lastError:
          "NEXT_PUBLIC_REALTIME_ENABLED=false — connect skipped (idle expected)",
      });
      return;
    }

    this.ensureNetworkListeners();

    if (!this.online) {
      this.setState("offline");
      return;
    }

    if (this.client) {
      if (this.state === "disconnected" || this.state === "error") {
        this.intentionalDisconnect = false;
        this.setState("connecting");
        this.client.connect();
      }
      return;
    }

    if (this.connectPromise) return this.connectPromise;

    this.connectPromise = this.doConnect().finally(() => {
      this.connectPromise = null;
    });
    return this.connectPromise;
  }

  private async doConnect() {
    this.intentionalDisconnect = false;
    this.setState("connecting");
    realtimeDebug.patch({ lastError: null });

    try {
      const { token, ws_url } = await this.fetchConnectionToken();
      const url = ws_url || realtimeConfig.wsUrl;
      this.wsUrl = url;
      realtimeDebug.patch({ wsUrl: url });

      this.client = new Centrifuge(url, {
        token,
        getToken: async () => {
          const next = await this.fetchConnectionToken();
          return next.token;
        },
        minReconnectDelay: realtimeConfig.reconnect.minDelayMs,
        maxReconnectDelay: realtimeConfig.reconnect.maxDelayMs,
      });

      this.client.on("connecting", (ctx) => {
        if (ctx.code === 0 && this.state === "connected") {
          this.setState("reconnecting");
        } else if (this.state !== "connecting") {
          this.setState("reconnecting");
        }
      });

      this.client.on("connected", () => {
        realtimeDebug.patch({ lastError: null });
        this.setState("connected");
      });

      this.client.on("disconnected", (ctx) => {
        if (this.intentionalDisconnect) {
          this.setState("idle");
          return;
        }
        this.reconnectCount += 1;
        this.reconnectListeners.forEach((l) => l(this.reconnectCount));
        realtimeDebug.incrementReconnect();
        const reason =
          typeof ctx?.reason === "string" && ctx.reason
            ? ctx.reason
            : `disconnected (code ${ctx?.code ?? "?"})`;
        realtimeDebug.patch({ lastError: reason });
        this.setState("disconnected");
      });

      this.client.on("error", (ctx) => {
        if (!this.intentionalDisconnect) {
          const reason =
            typeof ctx?.error?.message === "string" && ctx.error.message
              ? ctx.error.message
              : "Centrifugo error";
          realtimeDebug.patch({ lastError: reason });
          this.setState("error");
        }
      });

      this.client.connect();
    } catch (err) {
      this.client = null;
      const message = errorMessage(err);
      realtimeDebug.patch({ lastError: message });
      this.setState("error");
      throw err instanceof Error ? err : new Error(message);
    }
  }

  /**
   * Force token re-issue + reconnect (e.g. after API session cookie refresh).
   */
  async reconnectAfterAuthRefresh(): Promise<void> {
    if (this.intentionalDisconnect) return;
    if (!this.client) {
      await this.connect();
      return;
    }
    try {
      await this.client.setToken((await this.fetchConnectionToken()).token);
      this.client.connect();
    } catch (err) {
      realtimeDebug.patch({ lastError: errorMessage(err) });
      this.disconnect();
      await this.connect();
    }
  }

  disconnect() {
    this.intentionalDisconnect = true;
    this.client?.disconnect();
    this.client = null;
    this.wsUrl = null;
    this.tokenExpiresAt = null;
    this.userId = null;
    this.setState("idle");
    realtimeDebug.patch({
      wsUrl: null,
      tokenExpiresAt: null,
      userId: null,
      numericUserId: null,
      channels: [],
    });
  }
}

export const connectionManager = new ConnectionManager();
