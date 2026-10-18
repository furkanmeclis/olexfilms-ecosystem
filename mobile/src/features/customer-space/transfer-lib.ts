/** Pure helpers of the vehicle transfer flow (mirror of the web `vehicles/lib/transfer` and `portalTransferError`). */

type TransferLike = {
  status: string;
  from_verified: boolean;
  to_verified: boolean;
  new_owner_known: boolean;
};

/** The open transfer of a vehicle (the API allows one pending per vehicle). */
export function pendingTransfer<T extends { status: string }>(items: readonly T[] | undefined): T | undefined {
  return items?.find((t) => t.status === 'pending');
}

/** 6 digits, the format the API accepts for both codes. */
export function isTransferCode(value: string) {
  return /^[0-9]{6}$/.test(value.trim());
}

/** Digits only, at most 6 (what the code field keeps while typing). */
export function cleanCode(value: string) {
  return value.replace(/\D/g, '').slice(0, 6);
}

/** Loose phone check: optional +, 7 to 15 digits (the API validates the number itself). */
export function isTransferPhone(value: string) {
  return /^\+?[0-9]{7,15}$/.test(normalizePhone(value));
}

/** Phone as sent to the API: spaces and punctuation removed. */
export function normalizePhone(value: string) {
  return value.replace(/[\s()-]/g, '');
}

/**
 * The verify request body: only codes of sides that are still open and typed
 * in; the name only when the new owner has no account yet.
 */
export function verifyBody(
  transfer: TransferLike,
  input: { fromCode: string; toCode: string; name: string; surname: string },
) {
  const body: { from_code?: string; to_code?: string; new_owner_name?: string; new_owner_surname?: string } = {};
  const from = input.fromCode.trim();
  const to = input.toCode.trim();
  if (!transfer.from_verified && from) body.from_code = from;
  if (!transfer.to_verified && to) body.to_code = to;
  if (!transfer.new_owner_known) {
    if (input.name.trim()) body.new_owner_name = input.name.trim();
    if (input.surname.trim()) body.new_owner_surname = input.surname.trim();
  }
  return body;
}

/** Whether the verify form can be sent: a fresh valid code, and the name when it is now required. */
export function canVerify(transfer: TransferLike, input: { fromCode: string; toCode: string; name: string }) {
  const anyCode =
    (!transfer.from_verified && isTransferCode(input.fromCode)) || (!transfer.to_verified && isTransferCode(input.toCode));
  const fromReady = transfer.from_verified || isTransferCode(input.fromCode);
  const toReady = transfer.to_verified || isTransferCode(input.toCode);
  const needsName = !transfer.new_owner_known && fromReady && toReady;
  return anyCode && (!needsName || input.name.trim() !== '');
}

const ERROR_KEYS: Record<string, string> = {
  VEHICLE_TRANSFER_LOCKED: 'transfer_error.locked',
  VEHICLE_TRANSFER_EXPIRED: 'transfer_error.expired',
  VEHICLE_TRANSFER_NOT_PENDING: 'transfer_error.not_pending',
  VEHICLE_TRANSFER_PENDING: 'transfer_error.pending',
  VEHICLE_TRANSFER_SAME_OWNER: 'transfer_error.same_owner',
  VEHICLE_TRANSFER_OWNER_NO_PHONE: 'transfer_error.no_phone',
  VEHICLE_TRANSFER_DELIVERY_FAILED: 'transfer_error.delivery',
  VEHICLE_TRANSFER_UNAVAILABLE: 'transfer_error.unavailable',
  VEHICLE_TRANSFER_OWNER_CHANGED: 'transfer_error.not_pending',
  PORTAL_READ_ONLY: 'transfer_error.read_only',
};

/** Customer-namespace message key (and params) of a failed transfer request. */
export function transferErrorKey(err: {
  status?: number;
  code?: string | null;
  details?: { field?: string; code?: string }[];
}): { key: string; params?: Record<string, number> } {
  if (err.code === 'VEHICLE_TRANSFER_INVALID_CODE') {
    const left = Number(err.details?.[0]?.code);
    return { key: 'transfer_error.invalid_code', params: { count: Number.isFinite(left) ? left : 0 } };
  }
  if (err.code && ERROR_KEYS[err.code]) return { key: ERROR_KEYS[err.code] };
  if (err.code === 'VALIDATION_ERROR') {
    const field = err.details?.[0]?.field;
    if (field === 'phone') return { key: 'transfer_error.phone' };
    if (field === 'new_owner_name') return { key: 'transfer_error.name' };
    return { key: 'transfer_error.code_format' };
  }
  if (err.status === 429) return { key: 'transfer_error.rate' };
  if (err.status === 404) return { key: 'transfer_error.not_found' };
  return { key: 'transfer_error.generic' };
}
