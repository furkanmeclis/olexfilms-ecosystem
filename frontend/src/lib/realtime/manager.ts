import { channelManager } from "@/lib/realtime/channel-manager";
import { connectionManager } from "@/lib/realtime/connection-manager";
import { realtimeEventDispatcher } from "@/lib/realtime/event-dispatcher";
import type {
  RealtimeEventHandler,
  RealtimePublicationHandler,
} from "@/lib/realtime/types";

/**
 * Facade over the singleton connection + channel managers.
 * Prefer hooks from `@/hooks` in React; use this from non-React code only.
 */
class RealtimeManager {
  getStatus() {
    return connectionManager.getState();
  }

  subscribeStatus(
    listener: Parameters<typeof connectionManager.subscribeStatus>[0],
  ) {
    return connectionManager.subscribeStatus(listener);
  }

  subscribeOnline(
    listener: Parameters<typeof connectionManager.subscribeOnline>[0],
  ) {
    return connectionManager.subscribeOnline(listener);
  }

  subscribeReconnect(
    listener: Parameters<typeof connectionManager.subscribeReconnect>[0],
  ) {
    return connectionManager.subscribeReconnect(listener);
  }

  connect() {
    return connectionManager.connect();
  }

  disconnect() {
    channelManager.unsubscribeAll();
    connectionManager.disconnect();
  }

  reconnectAfterAuthRefresh() {
    return connectionManager.reconnectAfterAuthRefresh();
  }

  getUserId() {
    return connectionManager.getUserId();
  }

  /** @deprecated Prefer `getUserId`. */
  getNumericUserId() {
    return connectionManager.getUserId();
  }

  getActiveChannels() {
    return channelManager.getActiveChannels();
  }

  getReconnectCount() {
    return connectionManager.getReconnectCount();
  }

  async subscribe(channel: string, onPublication: RealtimePublicationHandler) {
    return channelManager.subscribe(channel, onPublication);
  }

  unsubscribe(channel: string) {
    channelManager.unsubscribe(channel);
  }

  onEvent(type: string | "*", handler: RealtimeEventHandler) {
    return realtimeEventDispatcher.on(type, handler);
  }

  presence(channel: string) {
    return channelManager.getPresence(channel);
  }

  watchPresence(
    channel: string,
    onUpdate: Parameters<typeof channelManager.watchPresence>[1],
  ) {
    return channelManager.watchPresence(channel, onUpdate);
  }
}

export const realtimeManager = new RealtimeManager();
