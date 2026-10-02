import type { components } from "@/generated/api";

/**
 * Warranty certificate PDF of a service (TEC-188). The API queues an export
 * job (worker-docs renders it with Gotenberg); the client polls the job and
 * downloads the file once it is completed. The panel and the portal use the
 * same flow over different endpoints (see certificate.service.ts).
 */
export type CertificateJob = components["schemas"]["ExportJob"];

export type CertificateFile = { blob: Blob; filename: string | null };

export type CertificateClient = {
  /** Queues the certificate job (locale overrides the user language). */
  request: (locale?: string) => Promise<CertificateJob>;
  /** Reads the job state. */
  get: (jobUuid: string) => Promise<CertificateJob>;
  /** Downloads the completed job's PDF. */
  download: (job: CertificateJob) => Promise<CertificateFile>;
};

/** API error code of a service without an active warranty (409). */
export const NO_ACTIVE_WARRANTY = "NO_ACTIVE_WARRANTY";

/** The job failed, expired or did not finish in time. */
export class CertificateJobError extends Error {
  constructor(
    message: string,
    readonly job: CertificateJob | null,
  ) {
    super(message);
    this.name = "CertificateJobError";
  }
}

export type WaitOptions = {
  intervalMs?: number;
  timeoutMs?: number;
  sleep?: (ms: number) => Promise<void>;
  now?: () => number;
};

const defaultSleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));

/** Polls the job until it is completed; failed / expired / timeout throw. */
export async function waitForCertificate(
  client: CertificateClient,
  job: CertificateJob,
  {
    intervalMs = 1500,
    timeoutMs = 90_000,
    sleep = defaultSleep,
    now = Date.now,
  }: WaitOptions = {},
): Promise<CertificateJob> {
  const deadline = now() + timeoutMs;
  let current = job;
  for (;;) {
    if (current.status === "completed") return current;
    if (current.status === "failed" || current.status === "expired") {
      throw new CertificateJobError(
        current.error ?? `certificate job ${current.status}`,
        current,
      );
    }
    if (now() >= deadline) {
      throw new CertificateJobError("certificate job timed out", current);
    }
    await sleep(intervalMs);
    current = await client.get(current.uuid);
  }
}

/** Requests, waits for and downloads the certificate. */
export async function fetchCertificate(
  client: CertificateClient,
  locale?: string,
  options?: WaitOptions,
): Promise<{ job: CertificateJob; file: CertificateFile }> {
  const queued = await client.request(locale);
  const job = await waitForCertificate(client, queued, options);
  const file = await client.download(job);
  return { job, file };
}

/** Download name: the server's Content-Disposition, else a stable name. */
export function certificateFilename(
  file: CertificateFile,
  job: CertificateJob,
): string {
  return file.filename ?? `warranty-certificate-${job.uuid.slice(0, 8)}.pdf`;
}

/** Whether an API error is the "no active warranty" conflict. */
export function isNoActiveWarranty(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    (error as { code?: unknown }).code === NO_ACTIVE_WARRANTY
  );
}
