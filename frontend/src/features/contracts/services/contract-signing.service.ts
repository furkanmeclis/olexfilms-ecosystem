import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Contract = Schemas["Contract"];
export type ContractSigner = Schemas["ContractSigner"];
export type ContractMedia = Schemas["ContractMedia"];
export type ContractOtp = Schemas["ContractOTP"];
export type ContractStatus = Schemas["ContractStatus"];

/** Contract media limit of the API (jpeg/png/webp, 12 MB). */
export const CONTRACT_MEDIA_MAX_BYTES = 12 * 1024 * 1024;
export const CONTRACT_MEDIA_TYPES = [
  "image/jpeg",
  "image/png",
  "image/webp",
] as const;

/** Customer signing window after the OTP is sent (backend: 30 minutes). */
export const CONTRACT_SIGN_WINDOW_MS = 30 * 60 * 1000;

const enc = encodeURIComponent;

/**
 * Intake contract signing (TEC-287): contract instance of a service,
 * customer OTP + canvas signature, staff signature, media and the executed
 * PDF. Every call goes through the BFF with the active organization.
 */
export const contractSigningService = {
  createForService(serviceUuid: string, locale?: string) {
    return platformRequest<Contract>(
      "POST",
      `/v1/services/${enc(serviceUuid)}/contract`,
      { body: locale ? { locale } : {} },
    );
  },
  get(uuid: string) {
    return platformRequest<Contract>("GET", `/v1/contracts/${enc(uuid)}`);
  },
  requestCustomerOtp(uuid: string) {
    return platformRequest<ContractOtp>(
      "POST",
      `/v1/contracts/${enc(uuid)}/signers/customer/otp`,
    );
  },
  signCustomer(uuid: string, body: { code?: string; signature_png?: string }) {
    return platformRequest<Contract>(
      "POST",
      `/v1/contracts/${enc(uuid)}/signers/customer/sign`,
      { body },
    );
  },
  signStaff(uuid: string, body: { signature_png?: string }) {
    return platformRequest<Contract>(
      "POST",
      `/v1/contracts/${enc(uuid)}/signers/staff/sign`,
      { body },
    );
  },
  addMedia(uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<ContractMedia>(
      "POST",
      `/v1/contracts/${enc(uuid)}/media`,
      form,
    );
  },
  deleteMedia(uuid: string, media: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/contracts/${enc(uuid)}/media/${enc(media)}`,
    );
  },
  /** Executed PDF; the API answers 404 until it is ready. */
  downloadPdf(uuid: string) {
    return platformDownloadFile(`/v1/contracts/${enc(uuid)}/pdf`);
  },
};

export const contractSigningKeys = {
  all: ["contracts"] as const,
  detail: (uuid: string) => ["contracts", "detail", uuid] as const,
};

/** Raw base64 of a canvas data URL (the API takes either form). */
export function pngBase64(dataUrl: string): string {
  const comma = dataUrl.indexOf(",");
  return comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
}

export function signerOf(
  contract: Pick<Contract, "signers">,
  role: ContractSigner["role"],
): ContractSigner | undefined {
  return contract.signers.find((s) => s.role === role);
}

/** Client check of a media file before upload; null when it is fine. */
export function mediaFileError(file: {
  type: string;
  size: number;
}): "type" | "size" | null {
  if (!(CONTRACT_MEDIA_TYPES as readonly string[]).includes(file.type)) {
    return "type";
  }
  return file.size > CONTRACT_MEDIA_MAX_BYTES ? "size" : null;
}
