import { describe, expect, it } from "vitest";

import { qrPollInterval, statusLabelKey, statusVariant } from "./status";

describe("whatsapp status helpers", () => {
  it("maps statuses to badge tones", () => {
    expect(statusVariant("connected")).toBe("success");
    expect(statusVariant("qr")).toBe("warning");
    expect(statusVariant("banned")).toBe("danger");
    expect(statusVariant("whatever")).toBe("secondary");
  });

  it("falls back to the unknown label", () => {
    expect(statusLabelKey("logged_out")).toBe(
      "integrations.whatsapp.status.logged_out",
    );
    expect(statusLabelKey("nope")).toBe("integrations.whatsapp.status.unknown");
  });

  it("polls the QR only while waiting for a scan", () => {
    expect(qrPollInterval(true, false)).toBe(2500);
    expect(qrPollInterval(true, true)).toBe(false);
    expect(qrPollInterval(false, false)).toBe(false);
  });
});
