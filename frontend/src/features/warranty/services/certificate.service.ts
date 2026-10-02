import type {
  CertificateClient,
  CertificateJob,
} from "@/features/warranty/lib/certificate";
import {
  PORTAL_API_BASE,
  portalRequest,
} from "@/features/portal/lib/portal-client";
import {
  parseContentDispositionFilename,
  platformDownloadFile,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

const enc = encodeURIComponent;

function body(locale?: string) {
  return locale ? { locale } : {};
}

/**
 * Panel client (TEC-188): POST /v1/services/{uuid}/warranty-certificate,
 * poll /v1/warranty-certificates/{job}, download its PDF. Needs
 * warranties.read in the active organization.
 */
export function panelCertificateClient(serviceUuid: string): CertificateClient {
  return {
    request: (locale) =>
      platformRequest<CertificateJob>(
        "POST",
        `/v1/services/${enc(serviceUuid)}/warranty-certificate`,
        { body: body(locale) },
      ),
    get: (jobUuid) =>
      platformRequest<CertificateJob>(
        "GET",
        `/v1/warranty-certificates/${enc(jobUuid)}`,
      ),
    download: (job) =>
      platformDownloadFile(
        `/v1/warranty-certificates/${enc(job.uuid)}/download`,
      ),
  };
}

/**
 * Portal client (TEC-188): the signed-in customer's certificate of the
 * warranties they hold; the job is polled and downloaded through
 * /v1/portal/exports (portal BFF only).
 */
export function portalCertificateClient(
  serviceUuid: string,
): CertificateClient {
  return {
    request: (locale) =>
      portalRequest<CertificateJob>(
        `portal/services/${enc(serviceUuid)}/warranty-certificate`,
        { method: "POST", body: body(locale) },
      ),
    get: (jobUuid) =>
      portalRequest<CertificateJob>(`portal/exports/${enc(jobUuid)}`),
    download: async (job) => {
      const response = await fetch(
        `${PORTAL_API_BASE}/portal/exports/${enc(job.uuid)}/download`,
        { credentials: "include", cache: "no-store" },
      );
      if (!response.ok) {
        throw new Error(`Download failed (${response.status})`);
      }
      return {
        blob: await response.blob(),
        filename: parseContentDispositionFilename(
          response.headers.get("Content-Disposition"),
        ),
      };
    },
  };
}
