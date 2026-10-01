import { describe, expect, it } from "vitest";

import {
  groupVariables,
  insertAt,
  placeholder,
  placeholdersIn,
  unknownPlaceholders,
} from "@/features/document-templates/lib/variables";
import type { DocumentVariable } from "@/features/document-templates/services/document-templates.service";

const vars: DocumentVariable[] = [
  {
    key: "company_name",
    type: "text",
    group: "company",
    label_tr: "Firma",
    label_en: "Company",
  },
  {
    key: "company_logo",
    type: "html",
    group: "company",
    label_tr: "Logo",
    label_en: "Logo",
  },
  {
    key: "plate",
    type: "text",
    group: "vehicle",
    label_tr: "Plaka",
    label_en: "Plate",
  },
];

describe("document template variables", () => {
  it("finds placeholders like the server", () => {
    expect(
      placeholdersIn("<p>{{ plate }} {{.Company_Name}} {{plate}}</p>"),
    ).toEqual(["company_name", "plate"]);
  });

  it("reports unknown placeholders", () => {
    expect(unknownPlaceholders("{{plate}} {{nope}}", vars)).toEqual(["nope"]);
  });

  it("groups in schema order", () => {
    expect(groupVariables(vars).map((g) => [g.group, g.items.length])).toEqual([
      ["company", 2],
      ["vehicle", 1],
    ]);
  });

  it("inserts at the caret", () => {
    expect(insertAt("ab", 1, 1, placeholder("plate"))).toEqual({
      value: "a{{plate}}b",
      caret: 10,
    });
    expect(insertAt("abc", 1, 2, "X")).toEqual({ value: "aXc", caret: 2 });
  });
});
