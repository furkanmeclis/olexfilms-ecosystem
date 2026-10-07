import type {
  Lead,
  LeadCreateInput,
  LeadPatchInput,
  LeadSource,
  LeadStatus,
  LeadTargetType,
  LeadTemperature,
} from "@/features/leads/services/leads.service";

export const LEAD_PAGE_SIZE = 20;
export const LEAD_TARGET_TYPES: LeadTargetType[] = [
  "customer",
  "dealer_candidate",
  "distributor_candidate",
];
/**
 * Target types a user may pick for a lead: a dealer never recruits a
 * distributor, so the distributor candidate option is center/distributor
 * only (TEC-319).
 */
export function leadTargetTypesFor(orgType: string | undefined) {
  return orgType === "dealer"
    ? LEAD_TARGET_TYPES.filter((x) => x !== "distributor_candidate")
    : LEAD_TARGET_TYPES;
}
export const LEAD_SOURCES: LeadSource[] = [
  "incoming_call",
  "outgoing_call",
  "walk_in",
  "whatsapp",
  "social",
  "referral",
  "website",
  "application_form",
  "other",
];
export const LEAD_TEMPERATURES: LeadTemperature[] = ["cold", "warm", "hot"];
export const LEAD_STATUSES: LeadStatus[] = [
  "new",
  "contacted",
  "quoted",
  "won",
  "lost",
];

export type LeadFormValues = {
  target_type: LeadTargetType;
  customer_user_id: string;
  vehicle_id: string;
  candidate_company_name: string;
  candidate_contact_name: string;
  candidate_phone_e164: string;
  candidate_email: string;
  country_id: string;
  province_id: string;
  district_id: string;
  source: LeadSource;
  temperature: LeadTemperature;
  follow_up_date: string;
  assignee_user_id: string;
  notes: string;
};

export function emptyLeadForm(): LeadFormValues {
  return {
    target_type: "customer",
    customer_user_id: "",
    vehicle_id: "",
    candidate_company_name: "",
    candidate_contact_name: "",
    candidate_phone_e164: "",
    candidate_email: "",
    country_id: "",
    province_id: "",
    district_id: "",
    source: "incoming_call",
    temperature: "warm",
    follow_up_date: "",
    assignee_user_id: "",
    notes: "",
  };
}

export function leadFormOf(lead: Lead): LeadFormValues {
  return {
    target_type: lead.target_type,
    customer_user_id: lead.customer_user_id
      ? String(lead.customer_user_id)
      : "",
    vehicle_id: lead.vehicle_id ? String(lead.vehicle_id) : "",
    candidate_company_name: lead.candidate_company_name ?? "",
    candidate_contact_name: lead.candidate_contact_name ?? "",
    candidate_phone_e164: lead.candidate_phone_e164 ?? "",
    candidate_email: lead.candidate_email ?? "",
    country_id: lead.country_id ? String(lead.country_id) : "",
    province_id: lead.province_id ? String(lead.province_id) : "",
    district_id: lead.district_id ? String(lead.district_id) : "",
    source: lead.source,
    temperature: lead.temperature,
    follow_up_date: toInputDate(lead.follow_up_date),
    assignee_user_id: lead.assignee_user_id
      ? String(lead.assignee_user_id)
      : "",
    notes: lead.notes ?? "",
  };
}

export function leadName(lead: Lead) {
  if (lead.target_type === "customer") {
    return lead.candidate_contact_name || `#${lead.customer_user_id ?? "—"}`;
  }
  return lead.candidate_company_name || lead.candidate_contact_name || "—";
}

export function validateLeadForm(values: LeadFormValues) {
  const errors: Record<string, string> = {};
  if (values.target_type === "customer") {
    if (!positive(values.customer_user_id)) {
      errors.customer_user_id = "leads.form.errors.customer_required";
    }
  } else {
    if (!values.candidate_company_name.trim()) {
      errors.candidate_company_name = "leads.form.errors.company_required";
    }
    if (!values.candidate_contact_name.trim()) {
      errors.candidate_contact_name = "leads.form.errors.contact_required";
    }
  }
  const phone = values.candidate_phone_e164.trim();
  if (phone && !/^\+[1-9]\d{6,14}$/.test(phone)) {
    errors.candidate_phone_e164 = "leads.form.errors.phone_invalid";
  }
  if (
    values.candidate_email &&
    !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(values.candidate_email)
  ) {
    errors.candidate_email = "leads.form.errors.email_invalid";
  }
  if (
    values.follow_up_date &&
    Number.isNaN(new Date(values.follow_up_date).getTime())
  ) {
    errors.follow_up_date = "leads.form.errors.follow_up_invalid";
  }
  if (values.assignee_user_id && !positive(values.assignee_user_id)) {
    errors.assignee_user_id = "leads.form.errors.assignee_invalid";
  }
  return errors;
}

export function createBody(values: LeadFormValues): LeadCreateInput {
  const common = bodyCommon(values);
  return {
    ...common,
    target_type: values.target_type,
  };
}

export function patchBody(
  original: Lead,
  values: LeadFormValues,
): LeadPatchInput {
  const next = bodyCommon(values);
  const body: LeadPatchInput = {};
  const current = leadFormOf(original);
  for (const key of Object.keys(next) as (keyof typeof next)[]) {
    if (
      String(values[key as keyof LeadFormValues] ?? "") !==
      String(current[key as keyof LeadFormValues] ?? "")
    ) {
      Object.assign(body, { [key]: next[key] });
    }
  }
  if (values.target_type !== current.target_type)
    body.target_type = values.target_type;
  return body;
}

function bodyCommon(values: LeadFormValues) {
  const customer = values.target_type === "customer";
  return {
    customer_user_id: customer ? numberOrNull(values.customer_user_id) : null,
    vehicle_id: customer ? numberOrNull(values.vehicle_id) : null,
    candidate_company_name: customer
      ? null
      : clean(values.candidate_company_name),
    candidate_contact_name: customer
      ? null
      : clean(values.candidate_contact_name),
    candidate_phone_e164: clean(values.candidate_phone_e164),
    candidate_email: clean(values.candidate_email),
    country_id: customer ? null : numberOrNull(values.country_id),
    province_id: customer ? null : numberOrNull(values.province_id),
    district_id: customer ? null : numberOrNull(values.district_id),
    source: values.source,
    temperature: values.temperature,
    follow_up_date: values.follow_up_date
      ? new Date(values.follow_up_date).toISOString()
      : null,
    assignee_user_id: numberOrNull(values.assignee_user_id),
    notes: values.notes.trim(),
  };
}

function clean(value: string) {
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

function numberOrNull(value: string) {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : null;
}

function positive(value: string) {
  return numberOrNull(value) !== null;
}

function toInputDate(value: string | undefined) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString().slice(0, 16);
}

export function leadStatusTone(status: LeadStatus) {
  if (status === "won") return "success" as const;
  if (status === "lost") return "danger" as const;
  if (status === "quoted") return "warning" as const;
  return "default" as const;
}

export function leadTemperatureTone(temperature: LeadTemperature) {
  if (temperature === "hot") return "danger" as const;
  if (temperature === "warm") return "warning" as const;
  return "default" as const;
}
