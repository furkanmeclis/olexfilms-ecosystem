import type {
  CustomerCreateInput,
  CustomerDetail,
  CustomerType,
  CustomerUpdateInput,
  Vehicle,
  VehicleCreateInput,
  VehicleUpdateInput,
} from "@/features/customers/services/customers.service";
import { matchPlate, normalizePlate } from "@/features/geo/lib/plate";
import { normalizeVin, validateVin } from "@/features/services/lib/vin";

/** Column limits of backend/internal/modules/customers/usecase/validate.go. */
export const NAME_MAX = 100;
export const COMPANY_MAX = 200;
export const TAX_OFFICE_MAX = 150;
export const EMAIL_MAX = 255;
export const PLATE_MAX = 20;

export type CustomerFormValues = {
  phone: string;
  name: string;
  surname: string;
  email: string;
  type: CustomerType;
  company_name: string;
  tax_office: string;
  /** Empty on edit keeps the stored (encrypted) value. */
  national_id: string;
  tax_no: string;
};

export type CustomerFormField = keyof CustomerFormValues;
export type CustomerFormErrors = Partial<Record<CustomerFormField, string>>;

export const EMPTY_CUSTOMER_FORM: CustomerFormValues = {
  phone: "",
  name: "",
  surname: "",
  email: "",
  type: "individual",
  company_name: "",
  tax_office: "",
  national_id: "",
  tax_no: "",
};

/** Edit form values from a stored customer; identity numbers stay empty. */
export function customerFormFromDetail(c: CustomerDetail): CustomerFormValues {
  return {
    phone: c.phone ?? "",
    name: c.name,
    surname: c.surname,
    email: c.email ?? "",
    type: c.type,
    company_name: c.company_name ?? "",
    tax_office: c.tax_office ?? "",
    national_id: "",
    tax_no: "",
  };
}

const squash = (v: string) => v.trim().replace(/\s+/g, " ");
const IDENTITY_RE = /^[0-9A-Z]{5,20}$/;
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** National id / tax number like normalizeIdentityNumber on the server. */
export function normalizeIdentity(raw: string): string {
  return raw.toLocaleUpperCase("en-US").replace(/[\s\-./]/g, "");
}

/**
 * Client checks matching the backend rules (name required and at most 100
 * characters, valid e-mail, 5-20 letter/digit identity numbers). The phone
 * is only required here: the server parses it to E.164 (K29) and answers
 * a field error when it cannot.
 */
export function validateCustomerForm(
  v: CustomerFormValues,
  mode: "create" | "edit",
): Record<string, string> {
  const e: Record<string, string> = {};
  if (mode === "create") {
    const digits = v.phone.replace(/\D/g, "");
    if (!v.phone.trim()) e.phone = "phone_required";
    else if (digits.length < 7 || digits.length > 15) e.phone = "phone_invalid";
  }
  if (!squash(v.name)) e.name = "name_required";
  else if (squash(v.name).length > NAME_MAX) e.name = "too_long";
  if (squash(v.surname).length > NAME_MAX) e.surname = "too_long";
  const email = v.email.trim();
  if (email && (email.length > EMAIL_MAX || !EMAIL_RE.test(email))) {
    e.email = "email_invalid";
  }
  if (squash(v.company_name).length > COMPANY_MAX) e.company_name = "too_long";
  if (v.type === "corporate" && !squash(v.company_name)) {
    e.company_name = "company_required";
  }
  if (squash(v.tax_office).length > TAX_OFFICE_MAX) e.tax_office = "too_long";
  for (const key of ["national_id", "tax_no"] as const) {
    const n = normalizeIdentity(v[key]);
    if (n && !IDENTITY_RE.test(n)) e[key] = "identity_invalid";
  }
  return e;
}

/** POST /v1/customers body; empty optional fields are left out. */
export function buildCustomerCreate(
  v: CustomerFormValues,
  locale?: string,
): CustomerCreateInput {
  const opt = (s: string) => (squash(s) ? squash(s) : undefined);
  const body: CustomerCreateInput = {
    phone: v.phone.trim(),
    name: squash(v.name),
    type: v.type,
  };
  const surname = opt(v.surname);
  if (surname) body.surname = surname;
  const email = v.email.trim().toLowerCase();
  if (email) body.email = email;
  if (locale) body.locale = locale;
  if (v.type === "corporate") {
    const company = opt(v.company_name);
    if (company) body.company_name = company;
    const office = opt(v.tax_office);
    if (office) body.tax_office = office;
  }
  const nid = normalizeIdentity(v.national_id);
  if (nid) body.national_id = nid;
  const tax = normalizeIdentity(v.tax_no);
  if (tax) body.tax_no = tax;
  return body;
}

/**
 * PATCH body: only the fields that changed; a cleared optional field is
 * sent as null. Identity numbers are write-only (the API returns the last
 * four characters): an empty input keeps the stored value.
 */
export function buildCustomerUpdate(
  v: CustomerFormValues,
  original: CustomerFormValues,
): CustomerUpdateInput {
  const body: CustomerUpdateInput = {};
  const name = squash(v.name);
  if (name !== squash(original.name)) body.name = name;
  if (v.type !== original.type) body.type = v.type;
  const nullable = (
    key: "surname" | "email" | "company_name" | "tax_office",
  ) => {
    const now = key === "email" ? v.email.trim().toLowerCase() : squash(v[key]);
    const before =
      key === "email"
        ? original.email.trim().toLowerCase()
        : squash(original[key]);
    if (now !== before) body[key] = now ? now : null;
  };
  nullable("surname");
  nullable("email");
  nullable("company_name");
  nullable("tax_office");
  const nid = normalizeIdentity(v.national_id);
  if (nid) body.national_id = nid;
  const tax = normalizeIdentity(v.tax_no);
  if (tax) body.tax_no = tax;
  return body;
}

// --- Vehicles ---------------------------------------------------------------

export type VehicleFormValues = {
  plate: string;
  plate_country: string;
  brand_uuid: string;
  brand_name: string;
  model_uuid: string;
  model_name: string;
  model_year: string;
  vin: string;
};

export const EMPTY_VEHICLE_FORM: VehicleFormValues = {
  plate: "",
  plate_country: "",
  brand_uuid: "",
  brand_name: "",
  model_uuid: "",
  model_name: "",
  model_year: "",
  vin: "",
};

export function vehicleFormFromVehicle(v: Vehicle): VehicleFormValues {
  return {
    plate: v.plate ?? "",
    plate_country: v.plate_country ?? "",
    brand_uuid: v.car_brand?.uuid ?? "",
    brand_name: v.car_brand?.name ?? "",
    model_uuid: v.car_model?.uuid ?? "",
    model_name: v.car_model?.name ?? "",
    model_year: v.model_year ? String(v.model_year) : "",
    vin: v.vin ?? "",
  };
}

/** Minimal plate format shape the check needs (GET /v1/plate-formats). */
export type PlateRule = {
  country_iso2: string;
  regex: string;
  example: string;
};

/**
 * Plate check of geo.ValidatePlate: the compact plate (upper case, no
 * separators) must match the regex of the plate country's active format.
 * A country without a loaded format is left to the server (its answer is
 * INVALID_PLATE / 404, shown on the field).
 */
export function plateError(
  plate: string,
  country: string,
  formats: PlateRule[] | undefined,
): "plate_required" | "plate_too_long" | "plate_invalid" | null {
  const compact = normalizePlate(plate);
  if (!compact) return "plate_required";
  if ([...compact].length > PLATE_MAX) return "plate_too_long";
  const iso = country.trim().toUpperCase();
  const format = formats?.find((f) => f.country_iso2 === iso);
  if (format && !matchPlate(format.regex, plate)) return "plate_invalid";
  return null;
}

export function validateVehicleForm(
  v: VehicleFormValues,
  formats: PlateRule[] | undefined,
  currentYear = new Date().getFullYear(),
): Record<string, string> {
  const e: Record<string, string> = {};
  const plate = plateError(v.plate, v.plate_country, formats);
  if (plate) e.plate = plate;
  if (!v.plate_country.trim()) e.plate_country = "country_required";
  const y = v.model_year.trim();
  if (
    y &&
    (!/^\d{4}$/.test(y) || Number(y) < 1900 || Number(y) > currentYear + 1)
  ) {
    e.model_year = "year_invalid";
  }
  if (validateVin(v.vin)) e.vin = "vin_invalid";
  return e;
}

export function buildVehicleCreate(
  customerUuid: string,
  v: VehicleFormValues,
): VehicleCreateInput {
  const vin = normalizeVin(v.vin);
  return {
    customer_uuid: customerUuid,
    plate: v.plate.trim(),
    plate_country: v.plate_country.trim().toUpperCase(),
    car_brand_uuid: v.brand_uuid || null,
    car_model_uuid: v.model_uuid || null,
    model_year: v.model_year.trim() ? Number(v.model_year) : null,
    ...(vin ? { vin } : {}),
  };
}

/** PATCH body of the changed vehicle fields (null clears). */
export function buildVehicleUpdate(
  v: VehicleFormValues,
  original: VehicleFormValues,
): VehicleUpdateInput {
  const body: VehicleUpdateInput = {};
  if (v.plate.trim() !== original.plate.trim()) body.plate = v.plate.trim();
  const country = v.plate_country.trim().toUpperCase();
  if (country !== original.plate_country.trim().toUpperCase()) {
    body.plate_country = country || null;
  }
  if (v.brand_uuid !== original.brand_uuid) {
    body.car_brand_uuid = v.brand_uuid || null;
  }
  if (v.model_uuid !== original.model_uuid) {
    body.car_model_uuid = v.model_uuid || null;
  }
  if (v.model_year.trim() !== original.model_year.trim()) {
    body.model_year = v.model_year.trim() ? Number(v.model_year) : null;
  }
  const vin = normalizeVin(v.vin);
  if (vin !== normalizeVin(original.vin)) body.vin = vin || null;
  return body;
}

/** Display name of a customer (the API masks anonymized ones). */
export function customerDisplayName(c: {
  name: string;
  surname: string;
  company_name?: string | null;
  type?: CustomerType;
}): string {
  const person = [c.name, c.surname].filter(Boolean).join(" ");
  return person || c.company_name || "—";
}
