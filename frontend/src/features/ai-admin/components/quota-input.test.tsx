// @vitest-environment jsdom
import { act, createElement, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
    format: { number: (v: number) => String(v) },
  }),
}));

import { QuotaInput } from "@/features/ai-admin/components/quota-input";
import { parseQuotaInput } from "@/features/ai-admin/lib/quota";
import { aiSettingsSchema } from "@/features/ai-admin/lib/settings-form";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let lastValue: number | null = null;

function Harness({ initial }: { initial: number | null }) {
  const [value, setValue] = useState<number | null>(initial);
  lastValue = value;
  return createElement(QuotaInput, { id: "quota", value, onChange: setValue });
}

function render(initial: number | null) {
  act(() => root.render(createElement(Harness, { initial })));
}

const input = () => container.querySelector("input") as HTMLInputElement;
const unlimitedBadge = () =>
  container.querySelector('[data-testid="quota-unlimited"]');

function type(value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  act(() => {
    setter?.call(input(), value);
    input().dispatchEvent(new Event("input", { bubbles: true }));
  });
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  lastValue = null;
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("QuotaInput", () => {
  it("cannot hold a negative value", () => {
    render(1000);
    type("-500");
    expect(lastValue).toBe(500);
    expect(input().value).toBe("500");

    type("-");
    expect(lastValue).toBeNull();
    expect(input().value).toBe("");

    const minus = new KeyboardEvent("keydown", {
      key: "-",
      bubbles: true,
      cancelable: true,
    });
    act(() => {
      input().dispatchEvent(minus);
    });
    expect(minus.defaultPrevented).toBe(true);
  });

  it('shows the "unlimited" label only for 0', () => {
    render(2_000_000);
    expect(unlimitedBadge()).toBeNull();
    type("0");
    expect(lastValue).toBe(0);
    expect(unlimitedBadge()?.textContent).toContain("ai_admin.quota.unlimited");
    type("15");
    expect(unlimitedBadge()).toBeNull();
  });
});

describe("quota parsing and validation", () => {
  it("parses typed text to a whole non-negative number", () => {
    expect(parseQuotaInput("-12")).toBe(12);
    expect(parseQuotaInput("1.5")).toBe(15);
    expect(parseQuotaInput("2 000 000")).toBe(2_000_000);
    expect(parseQuotaInput("")).toBeNull();
    expect(parseQuotaInput("abc")).toBeNull();
  });

  it("rejects a negative or missing quota in the settings form", () => {
    const base = {
      default_model: "claude-sonnet-5-5",
      fast_model: "claude-haiku-4-5",
      default_monthly_token_quota: 0,
      system_pool_monthly_quota: 0,
      tools: {},
      extra_instructions: "",
      knowledge_text: "",
    };
    expect(aiSettingsSchema.safeParse(base).success).toBe(true);

    const negative = aiSettingsSchema.safeParse({
      ...base,
      default_monthly_token_quota: -1,
    });
    expect(negative.success).toBe(false);
    expect(negative.error?.issues[0]?.message).toBe(
      "ai_admin.errors.quota_negative",
    );

    const missing = aiSettingsSchema.safeParse({
      ...base,
      system_pool_monthly_quota: null,
    });
    expect(missing.error?.issues[0]?.message).toBe(
      "ai_admin.errors.quota_required",
    );
  });

  it("rejects a knowledge text over 20 KB (bytes)", () => {
    const result = aiSettingsSchema.safeParse({
      default_model: "m",
      fast_model: "m",
      default_monthly_token_quota: 1,
      system_pool_monthly_quota: 1,
      tools: {},
      extra_instructions: "",
      // 2-byte characters: 10 241 × 2 bytes > 20 KB.
      knowledge_text: "ş".repeat(10_241),
    });
    expect(result.error?.issues[0]?.message).toBe(
      "ai_admin.errors.knowledge_too_long",
    );
  });
});
