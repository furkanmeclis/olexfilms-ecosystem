/**
 * Names the backend sends that have their own label in the `ai` namespace
 * (TEC-385 read tools, TEC-387 write tools, preview field keys and card
 * warnings). Anything else falls back to a generic label, so a new tool
 * never shows a raw i18n key.
 */
export const TOOL_LABELS = [
  "balance_summary",
  "get_order",
  "get_service",
  "list_appointments",
  "list_leads",
  "list_orders",
  "list_sub_organizations",
  "lookup_warranties",
  "my_tasks",
  "search_customers",
  "search_products",
  "search_services",
  "service_activity_summary",
  "service_pdf_link",
  "stock_summary",
  "stock_units",
] as const;

export const ACTION_LABELS = [
  "add_service_note",
  "cancel_appointment",
  "create_appointment",
  "create_lead",
  "create_order_draft",
  "create_task",
  "set_lead_follow_up",
] as const;

export const FIELD_LABELS = [
  "company_name",
  "contact_name",
  "customer",
  "date",
  "description",
  "due_date",
  "estimated_minutes",
  "follow_up_date",
  "item",
  "lead",
  "note",
  "notes",
  "phone",
  "plate",
  "priority",
  "reason",
  "service_no",
  "source",
  "starts_at",
  "subject_organization",
  "target_type",
  "temperature",
  "time",
  "title",
] as const;

export const WARNING_LABELS = ["order_draft_not_submitted"] as const;

const tools = new Set<string>(TOOL_LABELS);
const actions = new Set<string>(ACTION_LABELS);
const fields = new Set<string>(FIELD_LABELS);
const warnings = new Set<string>(WARNING_LABELS);

type T = (key: string, params?: Record<string, string | number>) => string;

export function toolLabel(t: T, name: string): string {
  return tools.has(name) ? t(`ai.tools.${name}`) : t("ai.chat.tool_generic");
}

export function actionLabel(t: T, name: string, summary?: string): string {
  if (actions.has(name)) return t(`ai.actions.${name}`);
  return summary || t("ai.card.title");
}

export function fieldLabel(t: T, key: string): string {
  return fields.has(key) ? t(`ai.actions.fields.${key}`) : key;
}

export function warningLabel(t: T, key: string): string | null {
  return warnings.has(key) ? t(`ai.actions.warnings.${key}`) : null;
}
