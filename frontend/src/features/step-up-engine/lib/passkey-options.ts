import { startAuthentication } from "@simplewebauthn/browser";

export type PasskeyOptionsPayload = {
  publicKey?: Parameters<typeof startAuthentication>[0];
  challenge?: string;
} & Record<string, unknown>;

/** go-webauthn returns `{ publicKey: {...} }`; simplewebauthn expects the inner object. */
export function normalizePasskeyRequestOptions(
  raw: PasskeyOptionsPayload,
): Parameters<typeof startAuthentication>[0] {
  if (raw.publicKey) {
    return raw.publicKey;
  }
  if (typeof raw.challenge === "string") {
    return raw as Parameters<typeof startAuthentication>[0];
  }
  throw new Error("Invalid passkey options payload");
}
