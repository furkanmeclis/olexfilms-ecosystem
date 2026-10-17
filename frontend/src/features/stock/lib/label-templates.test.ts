import { describe, expect, it } from "vitest";

import {
  emptyTemplateValues,
  labelTemplateSchema,
  logoImageError,
  qrSideMm,
  templateValues,
  toTemplateInput,
  type LabelTemplateValues,
} from "@/features/stock/lib/label-templates";
import type { LabelTemplate } from "@/features/stock/services/label-templates.service";

const t = (key: string) => key;
const schema = labelTemplateSchema(t);
const PNG = "data:image/png;base64,iVBORw0KGgo=";

function errors(values: Partial<LabelTemplateValues>) {
  const res = schema.safeParse({
    ...emptyTemplateValues(),
    name: "Shelf labels",
    ...values,
  });
  if (res.success) return {};
  return Object.fromEntries(
    res.error.issues.map((i) => [i.path.join("."), i.message]),
  );
}

describe("label template form (TEC-764)", () => {
  it("accepts the defaults of both kinds", () => {
    expect(errors({})).toEqual({});
    expect(errors(emptyTemplateValues("location"))).toMatchObject({
      name: "stock.label_templates.validation.required",
    });
    expect(
      errors({ ...emptyTemplateValues("location"), name: "Bins" }),
    ).toEqual({});
  });

  it("checks the name, the size and the columns like the backend", () => {
    expect(errors({ name: "  " })).toHaveProperty("name");
    expect(errors({ name: "x".repeat(101) })).toHaveProperty("name");
    expect(errors({ width_mm: "19" })).toHaveProperty("width_mm");
    expect(errors({ width_mm: "200.5" })).toHaveProperty("width_mm");
    expect(errors({ width_mm: "70.25" })).toHaveProperty("width_mm");
    expect(errors({ width_mm: "70.5" })).toEqual({});
    expect(errors({ height_mm: "9" })).toHaveProperty("height_mm");
    expect(errors({ height_mm: "10" })).toEqual({});
    expect(errors({ columns: "0" })).toHaveProperty("columns");
    expect(errors({ columns: "9" })).toHaveProperty("columns");
    expect(errors({ columns: "2.5" })).toHaveProperty("columns");
  });

  it("requires a QR code on location labels", () => {
    expect(errors({ kind: "location", symbology: "code128" })).toEqual({
      symbology: "stock.label_templates.validation.location_qr",
    });
  });

  it("requires the logo of the chosen logo mode", () => {
    expect(errors({ logo_mode: "text", logo_text: " " })).toHaveProperty(
      "logo_text",
    );
    expect(errors({ logo_mode: "text", logo_text: "OLEX" })).toEqual({});
    expect(errors({ logo_mode: "image", logo_image: "" })).toEqual({
      logo_image: "stock.label_templates.validation.logo_required",
    });
    expect(errors({ logo_mode: "image", logo_image: PNG })).toEqual({});
  });

  it("checks the logo data URI", () => {
    expect(logoImageError(PNG)).toBeNull();
    expect(logoImageError("data:image/jpeg;base64,/9j/4AAQ")).toBeNull();
    expect(logoImageError("data:image/gif;base64,R0lGOD")).toBe("logo_type");
    expect(logoImageError(`data:image/png;base64,${"A".repeat(200_001)}`)).toBe(
      "logo_size",
    );
  });

  it("round-trips a saved template and sends only the mode's logo", () => {
    const tpl: LabelTemplate = {
      uuid: "t1",
      name: "Rolls",
      kind: "unit",
      symbology: "code128",
      logo_mode: "text",
      logo_text: "OLEX",
      logo_image: null,
      width_mm: "70.0",
      height_mm: "37.5",
      columns: 3,
      show_name: true,
      show_code_text: false,
      is_default: true,
      active: true,
      created_at: "2026-10-01T09:00:00Z",
      updated_at: "2026-10-01T09:00:00Z",
    };
    const values = templateValues(tpl);
    expect(values).toMatchObject({ width_mm: "70", height_mm: "37.5" });
    expect(toTemplateInput(values)).toEqual({
      name: "Rolls",
      kind: "unit",
      symbology: "code128",
      logo_mode: "text",
      logo_text: "OLEX",
      width_mm: "70",
      height_mm: "37.5",
      columns: 3,
      show_name: true,
      show_code_text: false,
      is_default: true,
      active: true,
    });
    expect(
      toTemplateInput({ ...values, logo_mode: "none" }),
    ).not.toHaveProperty("logo_text");
  });

  it("sizes the preview QR like the printer", () => {
    expect(
      qrSideMm({
        width: 50,
        height: 50,
        showName: true,
        showCodeText: true,
        logo: false,
      }),
    ).toBe(36);
    expect(
      qrSideMm({
        width: 20,
        height: 10,
        showName: true,
        showCodeText: true,
        logo: true,
      }),
    ).toBe(10);
  });
});
