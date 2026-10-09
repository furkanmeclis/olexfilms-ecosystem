import { describe, expect, it } from "vitest";

import {
  canArchive,
  canVoid,
  invoicesCsv,
  seriesError,
  statusTimeline,
  validVoidReason,
} from "@/features/einvoice/lib/einvoice";
import {
  settingsBody,
  settingsDefaults,
  settingsFormSchema,
} from "@/features/einvoice/lib/settings-form";
import type { Einvoice } from "@/features/einvoice/services/einvoice.service";

const t = (key: string) => key;

function invoice(over: Partial<Einvoice> = {}): Einvoice {
  return {
    uuid: "11111111-1111-4111-8111-111111111111",
    number: null,
    profile: "EARSIVFATURA",
    invoice_type: "SATIS",
    status: "draft",
    validation_status: "valid",
    validation_messages: [],
    source_type: "order",
    source_uuid: "22222222-2222-4222-8222-222222222222",
    buyer_organization: { uuid: "o2", name: "Ege Dağıtım" },
    buyer: { name: "Ege Dağıtım", einvoice_registered: false },
    seller: { name: "Merkez", einvoice_registered: true },
    lines: [],
    tax_breakdown: [],
    currency: "TRY",
    line_extension: "300.00",
    tax_exclusive: "300.00",
    tax_total: "60.00",
    payable: "360.00",
    issue_date: "2026-10-01",
    xml_sha256: null,
    has_xml: false,
    has_pdf: false,
    error: null,
    voided_at: null,
    void_reason: null,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...over,
  };
}

const valid = {
  ...settingsDefaults(),
  vkn: "9000068418",
  tax_office: "Beşiktaş",
  legal_name: "Olexfilms Merkez A.Ş.",
  address: "Papatya Cad. No:21",
  city: "İstanbul",
  district: "Beşiktaş",
};

function seriesIssue(values: typeof valid, field: string) {
  const res = settingsFormSchema(t).safeParse(values);
  if (res.success) return null;
  return res.error.issues.find((i) => i.path[0] === field)?.message ?? null;
}

describe("e-invoice series (TEC-504)", () => {
  it("rejects a series that is not exactly 3 characters", () => {
    expect(seriesError("AB")).toBe("length");
    expect(seriesError("ABCD")).toBe("length");
    expect(seriesError("")).toBe("length");
    expect(seriesError("A-B")).toBe("chars");
    expect(seriesError("TMP")).toBe("reserved");
    expect(seriesError("EAR")).toBeNull();
  });

  it("fails the settings form outside 3 characters and passes a valid pair", () => {
    expect(
      seriesIssue({ ...valid, earchive_series: "EA" }, "earchive_series"),
    ).toBe("einvoice.settings.validation.series_length");
    expect(
      seriesIssue({ ...valid, efatura_series: "EFNX" }, "efatura_series"),
    ).toBe("einvoice.settings.validation.series_length");
    expect(
      seriesIssue({ ...valid, efatura_series: "TMP" }, "efatura_series"),
    ).toBe("einvoice.settings.validation.series_reserved");
    expect(
      seriesIssue(
        { ...valid, earchive_series: "OEA", efatura_series: "oea" },
        "efatura_series",
      ),
    ).toBe("einvoice.settings.validation.series_same");
    expect(settingsFormSchema(t).safeParse(valid).success).toBe(true);
  });

  it("uppercases the series and leaves empty optional fields out of the body", () => {
    const body = settingsBody({
      ...valid,
      earchive_series: "oea",
      efatura_series: "oef",
    });
    expect(body.earchive_series).toBe("OEA");
    expect(body.efatura_series).toBe("OEF");
    expect(body).not.toHaveProperty("iban");
    expect(body).not.toHaveProperty("mersis_no");
  });
});

describe("e-invoice actions (TEC-504)", () => {
  it("needs a void reason", () => {
    expect(validVoidReason("")).toBe(false);
    expect(validVoidReason("   ")).toBe(false);
    expect(validVoidReason("x".repeat(1001))).toBe(false);
    expect(validVoidReason("Yanlış tutar")).toBe(true);
  });

  it("closes archive for an invalid invoice", () => {
    expect(canArchive(invoice())).toBe(true);
    expect(
      canArchive(
        invoice({ status: "failed", validation_status: "rule_warnings" }),
      ),
    ).toBe(true);
    expect(
      canArchive(invoice({ status: "failed", validation_status: "invalid" })),
    ).toBe(false);
    expect(
      canArchive(invoice({ status: "draft", validation_status: "invalid" })),
    ).toBe(false);
    expect(
      canArchive(invoice({ status: "archived", number: "EAR2026000000001" })),
    ).toBe(false);
    expect(canVoid(invoice({ status: "voided" }))).toBe(false);
  });

  it("derives the status history", () => {
    expect(statusTimeline(invoice()).map((s) => s.id)).toEqual(["created"]);
    const voided = invoice({
      status: "voided",
      number: "EAR2026000000001",
      voided_at: "2026-10-02T09:00:00Z",
      void_reason: "Yanlış tutar",
    });
    const steps = statusTimeline(voided);
    expect(steps.map((s) => s.id)).toEqual(["created", "archived", "voided"]);
    expect(steps[2].note).toBe("Yanlış tutar");
  });

  it("writes the list CSV with quoted cells", () => {
    const csv = invoicesCsv([
      invoice({ buyer_organization: { uuid: "o2", name: 'Ege, "A"' } }),
    ]);
    const [header, row] = csv.split("\n");
    expect(header.startsWith("number,ettn,buyer")).toBe(true);
    expect(row).toContain('"Ege, ""A"""');
  });
});
