import type {
  CertificateClient,
  CertificateJob,
} from "@/features/warranty/lib/certificate";
import { platformDownloadFile } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

const enc = encodeURIComponent;

/**
 * Service PDF client (TEC-196): POST /v1/services/{uuid}/pdf queues the
 * export job, GET /v1/service-pdfs/{job} polls it and /download returns
 * the PDF. Same export-job flow as the warranty certificate (TEC-188), so
 * it plugs into `fetchCertificate`. Needs services.read.
 */
export function servicePdfClient(serviceUuid: string): CertificateClient {
  return {
    request: (locale) =>
      platformRequest<CertificateJob>(
        "POST",
        `/v1/services/${enc(serviceUuid)}/pdf`,
        { body: locale ? { locale } : {} },
      ),
    get: (jobUuid) =>
      platformRequest<CertificateJob>(
        "GET",
        `/v1/service-pdfs/${enc(jobUuid)}`,
      ),
    download: (job) =>
      platformDownloadFile(`/v1/service-pdfs/${enc(job.uuid)}/download`),
  };
}
