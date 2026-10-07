import { describe, expect, it, vi } from "vitest";

import { SUPPORTED_LOCALES } from "@/config/i18n";
import type { UpstreamFetcher } from "@/features/warranty/lib/public-warranty";
import { loadMessages } from "@/lib/i18n/messages";

import {
  applicationMessages,
  EMPTY_APPLICATION,
  fetchDealerApplicationConfig,
  normalizePhone,
  submitDealerApplication,
  toApplicationPayload,
  validateApplication,
} from "./dealer-application";

const valid = {
  ...EMPTY_APPLICATION,
  company_name: "Firma",
  contact_name: "Yetkili",
  phone: "0532 123 45 67",
  country_id: "1",
  kvkk_consent: true,
};

describe("dealer application validation (TEC-320)", () => {
  it("normalizes phones in the selected country", () => {
    expect(normalizePhone("0532 123 45 67", "TR")).toBe("+905321234567");
    expect(normalizePhone("030 12345678", "DE")).toBe("+493012345678");
    expect(normalizePhone("+90 532 123 45 67", null)).toBe("+905321234567");
    expect(normalizePhone("123", "TR")).toBeNull();
    expect(normalizePhone("0532 123 45 67", null)).toBeNull();
  });

  it("flags required fields, an invalid phone, e-mail and KVKK", () => {
    expect(validateApplication(valid, "TR")).toEqual({});
    const errors = validateApplication(
      {
        ...EMPTY_APPLICATION,
        phone: "12",
        email: "not-an-email",
        message: "x".repeat(5001),
      },
      "TR",
    );
    expect(errors.company_name?.key).toBe(
      "landing.dealer_application.errors.required",
    );
    expect(errors.country_id?.key).toBe(
      "landing.dealer_application.errors.required",
    );
    expect(errors.phone?.key).toBe(
      "landing.dealer_application.errors.phone_invalid",
    );
    expect(errors.email?.key).toBe(
      "landing.dealer_application.errors.email_invalid",
    );
    expect(errors.message).toEqual({
      key: "landing.dealer_application.errors.too_long",
      params: { max: 5000 },
    });
    expect(errors.kvkk_consent?.key).toBe(
      "landing.dealer_application.errors.kvkk_required",
    );
  });

  it("drops the district without a province and keeps the honeypot", () => {
    const payload = toApplicationPayload(
      { ...valid, district_id: "5", website: "bot.example" },
      "TR",
      "ar",
    );
    expect(payload).toMatchObject({
      phone: "+905321234567",
      country_id: 1,
      province_id: null,
      district_id: null,
      language: "ar",
      website: "bot.example",
    });
    expect(payload).not.toHaveProperty("email");
  });
});

describe("dealer application requests (TEC-320)", () => {
  function upstream(status: number, body = "") {
    return vi.fn(async () => ({
      status,
      headers: new Headers(),
      body: new TextEncoder().encode(body).buffer as ArrayBuffer,
    })) as unknown as UpstreamFetcher;
  }
  const request = { clientIp: null, forwardedHost: "olexfilms.app" };

  it("reads the config: open, closed (false or 404), error", async () => {
    const open = JSON.stringify({ data: { enabled: true } });
    const off = JSON.stringify({ data: { enabled: false } });
    expect(
      await fetchDealerApplicationConfig(request, upstream(200, open)),
    ).toBe("open");
    expect(
      await fetchDealerApplicationConfig(request, upstream(200, off)),
    ).toBe("closed");
    expect(await fetchDealerApplicationConfig(request, upstream(404))).toBe(
      "closed",
    );
    expect(await fetchDealerApplicationConfig(request, upstream(500))).toBe(
      "error",
    );
  });

  it("maps the submit answers", async () => {
    const reply = (status: number, body?: unknown, headers?: HeadersInit) =>
      vi.fn(
        async () =>
          new Response(body ? JSON.stringify(body) : null, {
            status,
            headers,
          }),
      ) as unknown as typeof fetch;
    const payload = toApplicationPayload(valid, "TR", "tr");
    expect(await submitDealerApplication(payload, reply(202))).toEqual({
      kind: "ok",
    });
    expect(await submitDealerApplication(payload, reply(404))).toEqual({
      kind: "closed",
    });
    expect(
      await submitDealerApplication(
        payload,
        reply(429, undefined, { "Retry-After": "60" }),
      ),
    ).toEqual({ kind: "rate_limited", retryAfter: 60 });
    expect(
      await submitDealerApplication(
        payload,
        reply(400, {
          error: { details: [{ field: "email" }, { field: "unknown" }] },
        }),
      ),
    ).toEqual({ kind: "validation", fields: ["email"] });
  });
});

describe("dealer application texts (TEC-320)", () => {
  it("are translated in every language", async () => {
    for (const locale of SUPPORTED_LOCALES) {
      await loadMessages(locale);
      const messages = applicationMessages(locale);
      expect(Object.keys(messages).length, locale).toBeGreaterThan(30);
      for (const [key, text] of Object.entries(messages))
        expect(text, `${locale} ${key}`).not.toBe(key);
    }
  });
});
