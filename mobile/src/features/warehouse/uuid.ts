/**
 * RFC 4122 version 4 UUID for idempotency keys (`request_uuid`,
 * `idempotency_key`). Math.random is enough here: the key only has to be
 * unique per retry window, it is not a secret. No crypto module needed.
 */
export function uuidv4(): string {
  const h = Array.from({ length: 32 }, () => Math.floor(Math.random() * 16));
  h[12] = 4;
  h[16] = (h[16] & 0x3) | 0x8;
  const s = h.map((n) => n.toString(16)).join('');
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`;
}

/**
 * Keeps one key per logical request: the same request retried after a
 * failure reuses its key, a different request (or one after success) gets a
 * new one.
 */
export function createRetryKey() {
  let last: { sig: string; key: string } | null = null;
  return {
    for(sig: string) {
      if (last?.sig !== sig) last = { sig, key: uuidv4() };
      return last.key;
    },
    done() {
      last = null;
    },
  };
}
