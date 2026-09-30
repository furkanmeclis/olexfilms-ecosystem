/** Connection lifecycle for the singleton Centrifugo client. */
export type RealtimeConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting"
  | "error"
  | "offline";

/** Domain event envelope published by the Go Realtime Broadcaster. */
export type RealtimeMessage<TPayload = unknown> = {
  type: string;
  data: {
    actor_id?: number;
    module?: string;
    description?: string;
    occurred_at?: string;
    payload?: TPayload;
    [key: string]: unknown;
  };
};

export type RealtimePublicationHandler = (
  message: RealtimeMessage,
  meta: { channel: string },
) => void;

export type RealtimeEventHandler = (
  message: RealtimeMessage,
  meta: { channel: string },
) => void;

export type RealtimeStatusListener = (state: RealtimeConnectionState) => void;

export type RealtimePresenceClient = {
  client: string;
  user: string;
  connInfo?: unknown;
  chanInfo?: unknown;
};

export type RealtimePresenceSnapshot = {
  clients: Record<string, RealtimePresenceClient>;
  stats: { numClients: number; numUsers: number } | null;
};

export type RealtimeDebugSnapshot = {
  state: RealtimeConnectionState;
  wsUrl: string | null;
  channels: string[];
  lastEvent: RealtimeMessage | null;
  lastEventAt: string | null;
  reconnectCount: number;
  online: boolean;
  /** Centrifugo JWT `sub` — App user UUID string (legacy numeric ok). */
  userId: string | null;
  /** @deprecated Alias of `userId` for older debug consumers. */
  numericUserId: string | null;
  tokenExpiresAt: string | null;
  /** Last connect/token failure message (dev panel). */
  lastError: string | null;
  /** Mirrors `NEXT_PUBLIC_REALTIME_ENABLED` (false → connect skipped, idle). */
  enabled: boolean;
};

export type ConnectionTokenResult = {
  token: string;
  expires_at: string;
  ws_url: string;
};

export type SubscriptionTokenResult = {
  token: string;
  channel: string;
  expires_at: string;
};
