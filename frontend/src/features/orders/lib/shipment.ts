import type { OrderShippingInput } from "@/features/orders/services/orders.service";

/** TEC-1228: a decimal typed with a comma or a dot; "" = not given. */
function decimal(raw: string): number | null | undefined {
  const v = raw.trim().replace(",", ".");
  if (v === "") return undefined;
  if (!/^\d+(\.\d{1,2})?$/.test(v)) return null;
  return Number(v);
}

function count(raw: string): number | null | undefined {
  const v = raw.trim();
  if (v === "") return undefined;
  if (!/^\d{1,4}$/.test(v) || Number(v) < 1) return null;
  return Number(v);
}

/** Whether the optional package count (1–9999), desi and weight are valid. */
export function shipmentDetailsValid(
  packages: string,
  desi: string,
  weight: string,
): boolean {
  return (
    count(packages) !== null &&
    decimal(desi) !== null &&
    decimal(weight) !== null
  );
}

/** The shipment details of the ship transition body (invalid ones dropped). */
export function shipmentDetails(
  packages: string,
  desi: string,
  weight: string,
): Pick<OrderShippingInput, "package_count" | "desi" | "weight_kg"> {
  const out: Pick<OrderShippingInput, "package_count" | "desi" | "weight_kg"> =
    {};
  const p = count(packages);
  const d = decimal(desi);
  const w = decimal(weight);
  if (p != null) out.package_count = p;
  if (d != null) out.desi = d;
  if (w != null) out.weight_kg = w;
  return out;
}
