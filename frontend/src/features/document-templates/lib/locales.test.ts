import { describe, expect, it } from "vitest";

import {
  DOCUMENT_KINDS,
  DOCUMENT_LANGUAGES,
} from "@/features/document-templates/services/document-templates.service";
import en from "@/locales/en/documents.json";
import tr from "@/locales/tr/documents.json";

// Variable groups of backend/internal/modules/documents/model (KindSpec).
const VARIABLE_GROUPS = [
  "company",
  "document",
  "customer",
  "vehicle",
  "service",
  "measurement",
  "contract",
  "order",
  "invoice",
  "warranty",
  "totals",
];

describe("documents locale keys", () => {
  for (const [name, dict] of [
    ["tr", tr],
    ["en", en],
  ] as const) {
    it(`${name} labels every kind, language and variable group`, () => {
      const keys = new Set(Object.keys(dict));
      for (const k of DOCUMENT_KINDS) expect(keys).toContain(`kinds.${k}`);
      for (const l of DOCUMENT_LANGUAGES)
        expect(keys).toContain(`languages.${l}`);
      for (const g of VARIABLE_GROUPS)
        expect(keys).toContain(`variable_groups.${g}`);
      for (const s of ["active", "draft", "superseded", "new"])
        expect(keys).toContain(`status.${s}`);
    });
  }
});
