import type { ContractTemplateVariable } from "@/features/contracts/services/contract-templates.service";

const PLACEHOLDER_RE = /\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}/g;

/** Sample values for the visual preview only (never sent to the API). */
export const SAMPLE_VALUES: Readonly<Record<string, string>> = {
  customer_name: "Jane Doe",
  customer_phone: "+90 555 000 00 00",
  customer_email: "jane.doe@example.com",
  plate: "34 ABC 123",
  vin: "WVWZZZ1JZXW000001",
  vehicle_label: "Volkswagen Golf 2021",
  service_no: "SRV-000123",
  service_name: "PPF Full Body",
  package: "Premium",
  start_date: "2026-01-15",
  end_date: "2031-01-15",
  price: "45.000,00 TRY",
  org_name: "Olex Center",
  org_phone: "+90 212 000 00 00",
  org_email: "center@example.com",
  org_address: "Istanbul",
  staff_name: "John Smith",
};

function escapeHtml(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * Fills `{{key}}` placeholders with escaped sample values. `today` uses the
 * given date; unknown keys stay visible as `{{key}}`.
 */
export function renderPreview(
  html: string,
  variables: readonly ContractTemplateVariable[],
  today: string,
): string {
  const known = new Map(variables.map((v) => [v.key, v]));
  return html.replace(PLACEHOLDER_RE, (whole, raw: string) => {
    const key = raw.toLowerCase();
    if (!known.has(key)) return whole;
    if (key === "today") return escapeHtml(today);
    return escapeHtml(SAMPLE_VALUES[key] ?? known.get(key)!.label_en);
  });
}

/** Distinct placeholders the API does not allow (save would be rejected). */
export function unknownVariables(
  html: string,
  variables: readonly ContractTemplateVariable[],
): string[] {
  const allowed = new Set(variables.map((v) => v.key));
  const out = new Set<string>();
  for (const m of html.matchAll(PLACEHOLDER_RE)) {
    const key = m[1].toLowerCase();
    if (!allowed.has(key)) out.add(key);
  }
  return [...out].sort();
}
