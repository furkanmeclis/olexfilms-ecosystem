import { describe, expect, it, vi } from "vitest";

import {
  CertificateJobError,
  certificateFilename,
  fetchCertificate,
  isNoActiveWarranty,
  waitForCertificate,
  type CertificateClient,
  type CertificateJob,
} from "./certificate";

const job = (
  status: CertificateJob["status"],
  extra: Partial<CertificateJob> = {},
): CertificateJob => ({
  uuid: "11111111-2222-3333-4444-555555555555",
  resource: "tenant.warranty.certificate",
  format: "pdf",
  status,
  row_count: 2,
  created_at: "2026-10-02T09:00:00Z",
  ...extra,
});

function client(
  states: CertificateJob[],
): CertificateClient & { gets: number } {
  const c = {
    gets: 0,
    request: vi.fn(async () => states[0]!),
    get: vi.fn(async () => {
      c.gets++;
      return states[Math.min(c.gets, states.length - 1)]!;
    }),
    download: vi.fn(async () => ({
      blob: new Blob(["%PDF"]),
      filename: "garanti.pdf",
    })),
  };
  return c;
}

const fast = { sleep: async () => undefined };

describe("warranty certificate flow", () => {
  it("polls until completed, then downloads", async () => {
    const c = client([job("queued"), job("processing"), job("completed")]);
    const { job: done, file } = await fetchCertificate(c, "ar", fast);
    expect(c.request).toHaveBeenCalledWith("ar");
    expect(c.gets).toBe(2);
    expect(done.status).toBe("completed");
    expect(c.download).toHaveBeenCalledWith(done);
    expect(certificateFilename(file, done)).toBe("garanti.pdf");
  });

  it("throws on a failed job and on timeout", async () => {
    await expect(
      waitForCertificate(
        client([job("failed", { error: "boom" })]),
        job("queued"),
        fast,
      ),
    ).rejects.toBeInstanceOf(CertificateJobError);
    let t = 0;
    await expect(
      waitForCertificate(client([job("queued")]), job("queued"), {
        sleep: async () => {
          t += 1000;
        },
        now: () => t,
        timeoutMs: 3000,
      }),
    ).rejects.toThrow("timed out");
  });

  it("names the file and recognises the 409 code", () => {
    expect(
      certificateFilename(
        { blob: new Blob([]), filename: null },
        job("completed"),
      ),
    ).toBe("warranty-certificate-11111111.pdf");
    expect(isNoActiveWarranty({ code: "NO_ACTIVE_WARRANTY" })).toBe(true);
    expect(isNoActiveWarranty(new Error("x"))).toBe(false);
  });
});
