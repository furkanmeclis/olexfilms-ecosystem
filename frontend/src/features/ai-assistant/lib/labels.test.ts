import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  ACTION_LABELS,
  FIELD_LABELS,
  TOOL_LABELS,
  WARNING_LABELS,
} from "./labels";

const LOCALES = path.resolve(__dirname, "../../../locales");

describe("assistant label keys", () => {
  const expected = [
    ...TOOL_LABELS.map((k) => `tools.${k}`),
    ...ACTION_LABELS.map((k) => `actions.${k}`),
    ...FIELD_LABELS.map((k) => `actions.fields.${k}`),
    ...WARNING_LABELS.map((k) => `actions.warnings.${k}`),
    ...["executing", "confirmed", "failed", "cancelled", "expired"].map(
      (s) => `card.status_${s}`,
    ),
    ...[
      "provider_error",
      "turn_limit",
      "refusal",
      "max_tokens",
      "quota_exceeded",
      "network",
      "generic",
    ].map((c) => `errors.${c}`),
  ];

  for (const locale of fs.readdirSync(LOCALES)) {
    const file = path.join(LOCALES, locale, "ai.json");
    if (!fs.existsSync(file)) continue;
    it(`${locale} has every dynamic key`, () => {
      const messages = JSON.parse(fs.readFileSync(file, "utf8")) as Record<
        string,
        string
      >;
      expect(expected.filter((k) => !messages[k])).toEqual([]);
    });
  }
});
