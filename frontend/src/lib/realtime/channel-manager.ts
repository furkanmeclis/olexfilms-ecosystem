import { SubscriptionState, type Subscription } from "centrifuge";

import { connectionManager } from "@/lib/realtime/connection-manager";
import { realtimeDebug } from "@/lib/realtime/debug";
import { realtimeEventDispatcher } from "@/lib/realtime/event-dispatcher";
import type {
  RealtimeMessage,
  RealtimePresenceSnapshot,
  RealtimePublicationHandler,
} from "@/lib/realtime/types";
import { parseRealtimeMessage } from "@/lib/realtime/utils";
import { realtimeService } from "@/services/realtime.service";

type ChannelEntry = {
  subscription: Subscription;
  listeners: Set<RealtimePublicationHandler>;
};

/**
 * Ref-counted channel subscriptions on the singleton connection.
 * Private channels use Centrifugo subscription JWTs
 * (`allow_subscribe_for_client: false`).
 */
export class ChannelManager {
  private channels = new Map<string, ChannelEntry>();
  private pending = new Map<string, Promise<ChannelEntry>>();

  getActiveChannels(): string[] {
    return [...this.channels.keys()];
  }

  async subscribe(
    channel: string,
    onPublication: RealtimePublicationHandler,
  ): Promise<() => void> {
    if (!channel) {
      throw new Error("Realtime channel is required");
    }

    await connectionManager.connect();
    const client = connectionManager.getClient();
    if (!client) {
      throw new Error("Realtime client unavailable");
    }

    let entry = this.channels.get(channel);
    if (!entry) {
      let create = this.pending.get(channel);
      if (!create) {
        create = this.createChannel(client, channel).finally(() => {
          this.pending.delete(channel);
        });
        this.pending.set(channel, create);
      }
      entry = await create;
    }

    entry.listeners.add(onPublication);

    return () => {
      this.removeListener(channel, onPublication);
    };
  }

  private async createChannel(
    client: NonNullable<ReturnType<typeof connectionManager.getClient>>,
    channel: string,
  ): Promise<ChannelEntry> {
    const existing = client.getSubscription(channel);
    let sub = existing;

    if (!sub) {
      const { token } = await realtimeService.subscriptionToken(channel);
      sub = client.newSubscription(channel, {
        token,
        getToken: async () => {
          const next = await realtimeService.subscriptionToken(channel);
          return next.token;
        },
      });
    }

    const entry: ChannelEntry = {
      subscription: sub,
      listeners: new Set(),
    };

    sub.on("publication", (ctx) => {
      const parsed = parseRealtimeMessage(ctx.data);
      if (!parsed) return;
      const message: RealtimeMessage = {
        type: parsed.type,
        data: parsed.data,
      };
      realtimeDebug.recordEvent(message);
      realtimeEventDispatcher.emit(message, channel);
      entry.listeners.forEach((listener) => listener(message, { channel }));
    });

    sub.on("error", (ctx) => {
      const detail =
        typeof ctx.error === "object" &&
        ctx.error &&
        "message" in ctx.error &&
        typeof (ctx.error as { message?: unknown }).message === "string"
          ? (ctx.error as { message: string }).message
          : "subscription error";
      realtimeDebug.patch({
        lastError: `Subscribe ${channel}: ${detail}`,
      });
    });

    if (
      sub.state !== SubscriptionState.Subscribed &&
      sub.state !== SubscriptionState.Subscribing
    ) {
      sub.subscribe();
    }

    try {
      await sub.ready();
      realtimeDebug.patch({ lastError: null });
    } catch (err) {
      const detail = err instanceof Error ? err.message : "subscription failed";
      realtimeDebug.patch({
        lastError: `Subscribe ${channel}: ${detail}`,
      });
      throw err;
    }

    this.channels.set(channel, entry);
    realtimeDebug.patch({ channels: this.getActiveChannels() });
    return entry;
  }

  private removeListener(
    channel: string,
    onPublication: RealtimePublicationHandler,
  ) {
    const entry = this.channels.get(channel);
    if (!entry) return;

    entry.listeners.delete(onPublication);
    if (entry.listeners.size > 0) return;

    entry.subscription.unsubscribe();
    connectionManager.getClient()?.removeSubscription(entry.subscription);
    this.channels.delete(channel);
    realtimeDebug.patch({ channels: this.getActiveChannels() });
  }

  unsubscribe(channel: string) {
    const entry = this.channels.get(channel);
    if (!entry) return;
    entry.subscription.unsubscribe();
    connectionManager.getClient()?.removeSubscription(entry.subscription);
    this.channels.delete(channel);
    realtimeDebug.patch({ channels: this.getActiveChannels() });
  }

  unsubscribeAll() {
    for (const channel of [...this.channels.keys()]) {
      this.unsubscribe(channel);
    }
  }

  /**
   * Presence for a subscribed channel (Centrifugo `presence` + join/leave).
   * Channel must already be subscribed.
   */
  async getPresence(channel: string): Promise<RealtimePresenceSnapshot> {
    const entry = this.channels.get(channel);
    if (!entry) {
      return { clients: {}, stats: null };
    }

    if (entry.subscription.state !== SubscriptionState.Subscribed) {
      await entry.subscription.ready();
    }

    const [presence, stats] = await Promise.all([
      entry.subscription.presence(),
      entry.subscription.presenceStats(),
    ]);

    const clients: RealtimePresenceSnapshot["clients"] = {};
    for (const [id, info] of Object.entries(presence.clients)) {
      clients[id] = {
        client: info.client,
        user: info.user,
        connInfo: info.connInfo,
        chanInfo: info.chanInfo,
      };
    }

    return {
      clients,
      stats: {
        numClients: stats.numClients,
        numUsers: stats.numUsers,
      },
    };
  }

  /**
   * Ensure subscription, emit presence snapshot, refresh on join/leave.
   */
  async watchPresence(
    channel: string,
    onUpdate: (snapshot: RealtimePresenceSnapshot) => void,
  ): Promise<() => void> {
    const keepAlive = await this.subscribe(channel, () => undefined);
    const entry = this.channels.get(channel);
    if (!entry) {
      keepAlive();
      throw new Error("Realtime channel unavailable for presence");
    }

    const emit = async () => {
      try {
        onUpdate(await this.getPresence(channel));
      } catch {
        // Presence may fail briefly during reconnect; keep last snapshot.
      }
    };

    await emit();

    const onJoinLeave = () => {
      void emit();
    };
    entry.subscription.on("join", onJoinLeave);
    entry.subscription.on("leave", onJoinLeave);

    return () => {
      entry.subscription.removeListener("join", onJoinLeave);
      entry.subscription.removeListener("leave", onJoinLeave);
      keepAlive();
    };
  }
}

export const channelManager = new ChannelManager();
