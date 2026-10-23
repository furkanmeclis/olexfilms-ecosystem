/**
 * Names the backend sends that have their own label in the `assistant`
 * namespace (read tools, write tools, preview field keys and card warnings).
 * Anything else falls back to a generic label, so a new tool never shows a
 * raw i18n key. Same lists as the web's ai-assistant/lib/labels.ts.
 */
export const TOOL_LABELS = [
  'balance_summary',
  'get_order',
  'get_service',
  'list_appointments',
  'list_leads',
  'list_orders',
  'list_sub_organizations',
  'lookup_warranties',
  'my_tasks',
  'search_customers',
  'search_products',
  'search_services',
  'service_activity_summary',
  'service_pdf_link',
  'stock_summary',
  'stock_units',
] as const;

export const ACTION_LABELS = [
  'add_service_note',
  'cancel_appointment',
  'create_appointment',
  'create_lead',
  'create_order_draft',
  'create_task',
  'set_lead_follow_up',
] as const;

export const FIELD_LABELS = [
  'company_name',
  'contact_name',
  'customer',
  'date',
  'description',
  'due_date',
  'estimated_minutes',
  'follow_up_date',
  'item',
  'lead',
  'note',
  'notes',
  'phone',
  'plate',
  'priority',
  'reason',
  'service_no',
  'source',
  'starts_at',
  'subject_organization',
  'target_type',
  'temperature',
  'time',
  'title',
] as const;

export const WARNING_LABELS = ['order_draft_not_submitted'] as const;

const tools = new Set<string>(TOOL_LABELS);
const actions = new Set<string>(ACTION_LABELS);
const fields = new Set<string>(FIELD_LABELS);
const warnings = new Set<string>(WARNING_LABELS);

type T = (key: string) => string;

export function toolLabel(t: T, name: string): string {
  return tools.has(name) ? t(`tools.${name}`) : t('chat.tool_generic');
}

export function actionLabel(t: T, name: string, summary?: string): string {
  if (actions.has(name)) return t(`actions.${name}`);
  return summary || t('card.title');
}

export function fieldLabel(t: T, key: string): string {
  return fields.has(key) ? t(`actions.fields.${key}`) : key;
}

export function warningLabel(t: T, key: string): string | null {
  return warnings.has(key) ? t(`actions.warnings.${key}`) : null;
}

/** Chat error codes with their own message (`errors.<code>`). */
const ERROR_LABELS = new Set(['provider_error', 'turn_limit', 'refusal', 'max_tokens', 'quota_exceeded', 'network']);

export function chatErrorLabel(t: T, code: string, message?: string): string {
  if (ERROR_LABELS.has(code)) return t(`errors.${code}`);
  return message || t('errors.generic');
}
