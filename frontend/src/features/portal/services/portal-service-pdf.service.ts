import type {
  CertificateClient,
  CertificateJob,
} from "@/features/warranty/lib/certificate";
import {
  PORTAL_API_BASE,
  portalRequest,
} from "@/features/portal/lib/portal-client";
import { parseContentDispositionFilename } from "@/lib/api/platform-form-request";

const enc = encodeURIComponent;

/**
 * Portal service PDF (TEC-239): GET /v1/portal/services/{uuid}/pdf answers
 * a completed render (200) or a queued / running job (202); the job is
 * polled and downloaded through /v1/portal/exports (portal BFF only). Same
 * export-job flow as the warranty certificate, so it plugs into
 * `fetchCertificate`.
 */
export function portalServicePdfClient(serviceUuid: string): CertificateClient {
  return {
    request: (locale) =>
      portalRequest<CertificateJob>(
        `portal/services/${enc(serviceUuid)}/pdf${
          locale ? `?locale=${enc(locale)}` : ""
        }`,
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
