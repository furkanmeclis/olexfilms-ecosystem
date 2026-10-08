import { describe, expect, it } from "vitest";

import {
  validateCertificatePDF,
  validateRejectReason,
} from "@/features/certificates/services/certificates.service";

describe("certificate form guards (TEC-482)", () => {
  it("rejects non-PDF uploads and oversized PDFs before calling the API", () => {
    expect(
      validateCertificatePDF({
        name: "certificate.png",
        type: "image/png",
        size: 1200,
      } as File),
    ).toBe("pdf_only");
    expect(
      validateCertificatePDF({
        name: "certificate.pdf",
        type: "application/pdf",
        size: 10 * 1024 * 1024 + 1,
      } as File),
    ).toBe("too_large");
    expect(
      validateCertificatePDF({
        name: "certificate.pdf",
        type: "application/pdf",
        size: 10 * 1024 * 1024,
      } as File),
    ).toBeNull();
  });

  it("requires a reason for reject decisions", () => {
    expect(validateRejectReason("")).toBe(false);
    expect(validateRejectReason("   ")).toBe(false);
    expect(validateRejectReason("Wrong person")).toBe(true);
  });
});
