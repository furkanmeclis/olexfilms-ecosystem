import type {
  LeadTargetType,
  Lead,
} from "@/features/leads/services/leads.service";
import type {
  LeadConvertInput,
  Quote,
  QuoteLineInput,
  QuoteLineType,
  QuoteStatus,
} from "@/features/leads/services/quotes.service";

/** One editable row of the draft quote editor. */
export type QuoteLineDraft = {
  key: string;
  line_type: QuoteLineType;
  product_uuid: string | null;
  service_catalog_item_uuid: string | null;
  description: string;
  quantity: string;
  unit_price: string;
  discount_amount: string;
  /** Backend line total of the saved row; null once the row is edited. */
  line_total: string | null;
};

let draftSeq = 0;
const nextKey = () => `line-${++draftSeq}`;

/** Only draft quotes are editable; sent and decided quotes are locked. */
export function quoteEditable(quote: Pick<Quote, "status">) {
  return quote.status === "draft";
}

export function linesOf(quote: Quote): QuoteLineDraft[] {
  return [...quote.lines]
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((l) => ({
      key: nextKey(),
      line_type: l.line_type,
      product_uuid: l.product_uuid ?? null,
      service_catalog_item_uuid: l.service_catalog_item_uuid ?? null,
      description: l.description,
      quantity: l.quantity,
      unit_price: l.unit_price,
      discount_amount: l.discount_amount,
      line_total: l.line_total,
    }));
}

export function newLine(
  line_type: QuoteLineType,
  ref: string,
  description: string,
): QuoteLineDraft {
  return {
    key: nextKey(),
    line_type,
    product_uuid: line_type === "product" ? ref : null,
    service_catalog_item_uuid: line_type === "catalog_service" ? ref : null,
    description,
    quantity: "1",
    unit_price: "",
    discount_amount: "0.00",
    line_total: null,
  };
}

/**
 * Lines body for PUT /v1/quotes/{uuid}/lines. Without pricing.sale.write
 * the unit price is never sent, so the backend applies the default price.
 */
export function linesBody(
  lines: QuoteLineDraft[],
  canOverridePrice: boolean,
): QuoteLineInput[] {
  return lines.map((l) => ({
    line_type: l.line_type,
    product_uuid: l.product_uuid,
    service_catalog_item_uuid: l.service_catalog_item_uuid,
    description: l.description.trim() || null,
    quantity: l.quantity.trim() || "1",
    unit_price:
      canOverridePrice && l.unit_price.trim() ? l.unit_price.trim() : null,
    discount_amount: l.discount_amount.trim() || "0.00",
  }));
}

const qtyRe = /^[0-9]{1,9}(\.[0-9]{1,3})?$/;
const moneyRe = /^[0-9]{1,16}(\.[0-9]{1,2})?$/;

/** Field errors keyed `<row key>.<field>` (i18n keys). */
export function validateLines(lines: QuoteLineDraft[]) {
  const errors: Record<string, string> = {};
  for (const l of lines) {
    if (!qtyRe.test(l.quantity.trim()) || Number(l.quantity) <= 0) {
      errors[`${l.key}.quantity`] = "leads.quotes.errors.quantity";
    }
    if (l.unit_price.trim() && !moneyRe.test(l.unit_price.trim())) {
      errors[`${l.key}.unit_price`] = "leads.quotes.errors.money";
    }
    if (l.discount_amount.trim() && !moneyRe.test(l.discount_amount.trim())) {
      errors[`${l.key}.discount_amount`] = "leads.quotes.errors.money";
    }
  }
  return errors;
}

export function toDateInput(value: string | null | undefined) {
  if (!value) return "";
  return value.slice(0, 10);
}

export function fromDateInput(value: string) {
  return value ? new Date(`${value}T00:00:00Z`).toISOString() : null;
}

export function quoteStatusTone(status: QuoteStatus) {
  if (status === "accepted") return "success" as const;
  if (status === "rejected" || status === "expired") return "danger" as const;
  if (status === "sent") return "warning" as const;
  return "default" as const;
}

export type ConvertContext = {
  orgType: string | undefined;
  isSuperAdmin: boolean;
  canConvertOrg: boolean;
};

/**
 * Conversion targets the caller may use (mirrors the backend
 * authorizeConvert): customers always; dealer candidates need
 * leads.convert_org; distributor candidates only a center super_admin.
 * A dealer user therefore never gets the distributor option.
 */
export function convertKinds(ctx: ConvertContext): LeadTargetType[] {
  const kinds: LeadTargetType[] = ["customer"];
  if (ctx.canConvertOrg && ctx.orgType !== "dealer") {
    kinds.push("dealer_candidate");
  }
  if (ctx.orgType === "center" && ctx.isSuperAdmin) {
    kinds.push("distributor_candidate");
  }
  return kinds;
}

export function leadConverted(lead: Lead) {
  return lead.status === "won" || Boolean(lead.won_ref_type);
}

export type ConvertForm = {
  distributor_uuid: string;
  currency: string;
  register_as_warehouse: boolean;
};

export function convertBody(
  kind: LeadTargetType,
  form: ConvertForm,
  center: boolean,
): LeadConvertInput {
  if (kind === "dealer_candidate") {
    return {
      kind,
      distributor_uuid: center ? form.distributor_uuid || null : null,
    };
  }
  if (kind === "distributor_candidate") {
    return {
      kind,
      currency: form.currency.trim().toUpperCase(),
      register_as_warehouse: form.register_as_warehouse,
    };
  }
  return { kind };
}

export function validateConvert(
  kind: LeadTargetType,
  form: ConvertForm,
  center: boolean,
) {
  const errors: Record<string, string> = {};
  if (kind === "dealer_candidate" && center && !form.distributor_uuid) {
    errors.distributor_uuid = "leads.convert.errors.distributor_required";
  }
  if (
    kind === "distributor_candidate" &&
    !/^[A-Za-z]{3}$/.test(form.currency.trim())
  ) {
    errors.currency = "leads.convert.errors.currency_required";
  }
  return errors;
}
