import { platformDownloadRequest } from "@/lib/api/platform-form-request";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type LocalizedText = Record<string, string>;

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type CertificateStatus =
  "pending" | "valid" | "rejected" | "revoked" | "expired";
export type WarningDecision =
  "none" | "pending_approval" | "approved" | "rejected";

export type CertificateType = {
  uuid: string;
  name: LocalizedText;
  description?: LocalizedText | null;
  validity_months?: number | null;
  active: boolean;
  sort_order: number;
  category_count?: number;
  product_count?: number;
  created_at?: string;
};

export type CertificateTypeInput = {
  name: LocalizedText;
  description?: LocalizedText;
  validity_months?: number | null;
  active?: boolean;
  sort_order?: number;
  category_uuids?: string[];
  product_uuids?: string[];
};

export type Certificate = {
  uuid: string;
  user_uuid?: string;
  user_name?: string;
  user_surname?: string;
  type_uuid?: string;
  type_name?: LocalizedText;
  organization_uuid?: string;
  organization_name?: string;
  status: CertificateStatus | string;
  issued_at?: string | null;
  expires_at?: string | null;
  verified_at?: string | null;
  verified_by_user_name?: string | null;
  reject_reason?: string | null;
  created_at: string;
};

export type CertificateWarning = {
  uuid: string;
  service_uuid?: string;
  service_no?: string;
  user_uuid?: string;
  user_name?: string;
  user_surname?: string;
  type_uuid?: string;
  type_name?: LocalizedText;
  organization_uuid?: string;
  organization_name?: string;
  reason: string;
  decision: WarningDecision | string;
  note?: string | null;
  created_at: string;
  decided_at?: string | null;
};

export type CertificateServiceWarning = {
  uuid: string;
  reason: string;
  decision: WarningDecision | string;
  type?: { uuid: string; name: LocalizedText };
  user?: { uuid: string; name: string; surname?: string };
  note?: string | null;
  created_at?: string;
};

export type CertificateListQuery = Record<string, string | number | undefined>;

export const CERTIFICATE_STATUSES: CertificateStatus[] = [
  "pending",
  "valid",
  "rejected",
  "revoked",
  "expired",
];

export const WARNING_DECISIONS: WarningDecision[] = [
  "none",
  "pending_approval",
  "approved",
  "rejected",
];

const enc = encodeURIComponent;

function compact(params: CertificateListQuery) {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ""),
  ) as CertificateListQuery;
}

export function localizedName(
  value: LocalizedText | string | null | undefined,
  locale: string,
) {
  if (!value) return "";
  if (typeof value === "string") return value;
  return (
    value[locale] ??
    value[locale.replace("-", "_")] ??
    value.tr ??
    value.en ??
    Object.values(value)[0] ??
    ""
  );
}

export function personName(row: {
  user_name?: string | null;
  user_surname?: string | null;
}) {
  return [row.user_name, row.user_surname].filter(Boolean).join(" ");
}

export function validateCertificatePDF(
  file: Pick<File, "name" | "size" | "type"> | null,
) {
  if (!file) return "file_required";
  const pdf =
    file.type === "application/pdf" || file.name.toLowerCase().endsWith(".pdf");
  if (!pdf) return "pdf_only";
  if (file.size > 10 * 1024 * 1024) return "too_large";
  return null;
}

export function validateRejectReason(reason: string) {
  return reason.trim().length > 0;
}

export const certificateKeys = {
  all: ["certificates"] as const,
  types: (params: CertificateListQuery) =>
    [...certificateKeys.all, "types", params] as const,
  list: (params: CertificateListQuery) =>
    [...certificateKeys.all, "list", params] as const,
  warnings: (params: CertificateListQuery) =>
    [...certificateKeys.all, "warnings", params] as const,
};

export const certificatesService = {
  listTypes(params: CertificateListQuery) {
    return platformRequest<Page<CertificateType>>(
      "GET",
      "/v1/platform/certificate-types",
      { query: compact(params) },
    );
  },
  createType(body: CertificateTypeInput) {
    return platformRequest<CertificateType>(
      "POST",
      "/v1/platform/certificate-types",
      { body },
    );
  },
  updateType(uuid: string, body: CertificateTypeInput) {
    return platformRequest<CertificateType>(
      "PUT",
      `/v1/platform/certificate-types/${enc(uuid)}`,
      { body },
    );
  },
  deleteType(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/platform/certificate-types/${enc(uuid)}`,
    );
  },
  listCertificates(params: CertificateListQuery) {
    return platformRequest<Page<Certificate>>("GET", "/v1/certificates", {
      query: compact(params),
    });
  },
  uploadCertificate(input: {
    userUuid: string;
    typeUuid: string;
    issuedAt?: string;
    expiresAt?: string;
    file: File;
  }) {
    const form = new FormData();
    form.set("user_uuid", input.userUuid);
    form.set("type_uuid", input.typeUuid);
    if (input.issuedAt)
      form.set("issued_at", new Date(input.issuedAt).toISOString());
    if (input.expiresAt)
      form.set("expires_at", new Date(input.expiresAt).toISOString());
    form.set("file", input.file);
    return platformFormRequest<Certificate>("POST", "/v1/certificates", form);
  },
  downloadFile(uuid: string) {
    return platformDownloadRequest(`/v1/certificates/${enc(uuid)}/file`);
  },
  verify(uuid: string) {
    return platformRequest<Certificate>(
      "POST",
      `/v1/certificates/${enc(uuid)}/verify`,
    );
  },
  reject(uuid: string, reason: string) {
    const form = new FormData();
    form.set("reason", reason);
    return platformFormRequest<Certificate>(
      "POST",
      `/v1/certificates/${enc(uuid)}/reject`,
      form,
    );
  },
  revoke(uuid: string) {
    return platformRequest<Certificate>(
      "POST",
      `/v1/certificates/${enc(uuid)}/revoke`,
    );
  },
  listWarnings(params: CertificateListQuery) {
    return platformRequest<Page<CertificateWarning>>(
      "GET",
      "/v1/certificate-warnings",
      { query: compact(params) },
    );
  },
  approveWarning(uuid: string, note?: string) {
    const form = new FormData();
    if (note) form.set("note", note);
    return platformFormRequest<CertificateWarning>(
      "POST",
      `/v1/certificate-warnings/${enc(uuid)}/approve`,
      form,
    );
  },
  rejectWarning(uuid: string, note: string) {
    const form = new FormData();
    form.set("note", note);
    return platformFormRequest<CertificateWarning>(
      "POST",
      `/v1/certificate-warnings/${enc(uuid)}/reject`,
      form,
    );
  },
};
