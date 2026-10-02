export type TransferTone = "default" | "success" | "warning" | "danger";

type TransferLike = {
  status: string;
  from_verified: boolean;
  to_verified: boolean;
  new_owner_known: boolean;
};

/** The open transfer of a vehicle (the API allows one pending per vehicle). */
export function pendingTransfer<T extends { status: string }>(
  items: readonly T[] | undefined,
): T | undefined {
  return items?.find((t) => t.status === "pending");
}

/** 6 digits, the format the API accepts for both codes. */
export function isTransferCode(value: string): boolean {
  return /^[0-9]{6}$/.test(value.trim());
}

/**
 * The verify request body: only codes of sides that are still open and
 * typed in; the name only when the new owner has no account yet.
 */
export function verifyBody(
  transfer: TransferLike,
  input: { fromCode: string; toCode: string; name: string; surname: string },
): {
  from_code?: string;
  to_code?: string;
  new_owner_name?: string;
  new_owner_surname?: string;
} {
  const body: ReturnType<typeof verifyBody> = {};
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

export function transferStatusTone(status: string): TransferTone {
  if (status === "completed") return "success";
  if (status === "pending") return "warning";
  return "default";
}
