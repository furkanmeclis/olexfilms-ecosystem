import { describe, expect, it } from "vitest";

import {
  renderPreview,
  SAMPLE_VALUES,
  unknownVariables,
} from "@/features/contracts/lib/preview";

const variables = [
  {
    key: "customer_name",
    group: "customer",
    label_tr: "Müşteri adı",
    label_en: "Customer name",
  },
  { key: "plate", group: "vehicle", label_tr: "Plaka", label_en: "Plate" },
  { key: "today", group: "date", label_tr: "Bugün", label_en: "Today" },
];

describe("contract template preview (TEC-290)", () => {
  it("fills known placeholders with escaped sample values", () => {
    const html =
      "<p>{{customer_name}} · {{ PLATE }} · {{today}} · {{nope}}</p>";
    expect(renderPreview(html, variables, "05.10.2026")).toBe(
      `<p>${SAMPLE_VALUES.customer_name} · ${SAMPLE_VALUES.plate} · 05.10.2026 · {{nope}}</p>`,
    );
    expect(renderPreview("{{today}}", variables, "<b>x</b>")).toBe(
      "&lt;b&gt;x&lt;/b&gt;",
    );
  });

  it("lists placeholders the API would reject", () => {
    expect(
      unknownVariables("{{plate}} {{foo}} {{Bar}} {{foo}}", variables),
    ).toEqual(["bar", "foo"]);
  });
});
