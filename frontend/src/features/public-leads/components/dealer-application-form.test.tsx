// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { registerMessages, translate } from "@/lib/i18n/messages";
import tr from "@/locales/tr";

import { applicationMessages } from "../lib/dealer-application";
import { DealerApplicationForm } from "./dealer-application-form";

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

// Radix Checkbox measures itself; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

const COUNTRIES = [
  {
    id: 1,
    iso2: "TR",
    iso3: "TUR",
    name_en: "Turkey",
    name_tr: "Türkiye",
    is_active: true,
    has_provinces: true,
  },
  {
    id: 2,
    iso2: "DE",
    iso3: "DEU",
    name_en: "Germany",
    name_tr: "Almanya",
    is_active: true,
    has_provinces: false,
  },
];
const PROVINCES = [
  { id: 34, country_id: 1, code: "34", name: "İstanbul", has_districts: true },
  { id: 6, country_id: 1, code: "06", name: "Ankara", has_districts: false },
];
const DISTRICTS = [{ id: 501, province_id: 34, name: "Kadıköy" }];

type Reply = { status: number; body?: unknown; headers?: HeadersInit };

let submitReply: Reply;
let calls: { url: string; init?: RequestInit }[];

const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  calls.push({ url, init });
  const json = (r: Reply) =>
    new Response(r.body === undefined ? null : JSON.stringify(r.body), {
      status: r.status,
      headers: r.headers,
    });
  if (url === "/api/v1/public/geo/countries")
    return json({ status: 200, body: { data: { items: COUNTRIES } } });
  if (url === "/api/v1/public/geo/countries/TR/provinces")
    return json({ status: 200, body: { data: { items: PROVINCES } } });
  if (url === "/api/v1/public/geo/provinces/34/districts")
    return json({ status: 200, body: { data: { items: DISTRICTS } } });
  if (url === "/api/v1/public/dealer-applications") return json(submitReply);
  return json({ status: 404 });
}) as unknown as typeof fetch;

let container: HTMLDivElement;
let root: Root;

const msg = (key: string, params?: Record<string, string | number>) =>
  translate("tr", `landing.dealer_application.${key}`, params);

beforeAll(() => {
  registerMessages("tr", tr);
});

beforeEach(() => {
  calls = [];
  submitReply = { status: 202, body: { data: { received: true } } };
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  await act(async () => {
    root.render(
      createElement(DealerApplicationForm, {
        locale: "tr",
        messages: applicationMessages("tr"),
        homeHref: "/?lang=tr",
        fetchImpl: fetchMock,
      }),
    );
  });
  await flush();
}

function field<T extends HTMLElement>(name: string): T {
  const el = container.querySelector<T>(`[name="${name}"]`);
  if (!el) throw new Error(`field ${name} not rendered`);
  return el;
}

async function type(name: string, value: string) {
  const el = field<HTMLInputElement | HTMLTextAreaElement>(name);
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function choose(name: string, value: string) {
  const el = field<HTMLSelectElement>(name);
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLSelectElement.prototype,
      "value",
    )!.set!.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

function submitButton() {
  return container.querySelector<HTMLButtonElement>("button[type='submit']")!;
}

async function tickKvkk() {
  await act(async () => {
    container.querySelector<HTMLElement>("[role='checkbox']")!.click();
  });
}

async function submit() {
  await act(async () => {
    submitButton().click();
  });
  await flush();
}

function posted() {
  return calls.filter((c) => c.url === "/api/v1/public/dealer-applications");
}

async function fillValid(phone = "0532 123 45 67") {
  await type("company_name", "Kadıköy Kaplama");
  await type("contact_name", "Ayşe Yılmaz");
  await choose("country_id", "1");
  await type("phone", phone);
}

describe("DealerApplicationForm (TEC-320)", () => {
  it("keeps submit disabled until the KVKK consent is ticked", async () => {
    await render();
    await fillValid();
    expect(submitButton().disabled).toBe(true);
    await tickKvkk();
    expect(submitButton().disabled).toBe(false);
    await tickKvkk();
    expect(submitButton().disabled).toBe(true);
  });

  it("loads the provinces when a country is chosen, then the districts", async () => {
    await render();
    const province = field<HTMLSelectElement>("province_id");
    expect(province.disabled).toBe(true);
    expect(calls.map((c) => c.url)).toEqual(["/api/v1/public/geo/countries"]);

    await choose("country_id", "1");
    expect(calls.map((c) => c.url)).toContain(
      "/api/v1/public/geo/countries/TR/provinces",
    );
    expect(province.disabled).toBe(false);
    expect([...province.options].map((o) => o.textContent)).toEqual([
      msg("province_placeholder"),
      "İstanbul",
      "Ankara",
    ]);

    const district = field<HTMLSelectElement>("district_id");
    expect(district.disabled).toBe(true);
    await choose("province_id", "34");
    expect(district.disabled).toBe(false);
    expect([...district.options].map((o) => o.textContent)).toContain(
      "Kadıköy",
    );

    // A country without provinces leaves both pickers empty and disabled.
    await choose("country_id", "2");
    expect(province.disabled).toBe(true);
    expect(province.options).toHaveLength(1);
    expect(district.disabled).toBe(true);
  });

  it("shows an invalid phone error and does not post", async () => {
    await render();
    await fillValid("123");
    await tickKvkk();
    await submit();
    const phone = field<HTMLInputElement>("phone");
    expect(phone.getAttribute("aria-invalid")).toBe("true");
    expect(container.textContent).toContain(msg("errors.phone_invalid"));
    expect(posted()).toHaveLength(0);
  });

  it("posts the application with the phone in E.164 and shows the success screen", async () => {
    await render();
    await fillValid();
    await choose("province_id", "34");
    await choose("district_id", "501");
    await type("email", "ayse@example.com");
    await type("message", "Merhaba");
    await tickKvkk();
    await submit();
    expect(posted()).toHaveLength(1);
    const body = JSON.parse(String(posted()[0]!.init?.body));
    expect(body).toEqual({
      company_name: "Kadıköy Kaplama",
      contact_name: "Ayşe Yılmaz",
      phone: "+905321234567",
      email: "ayse@example.com",
      message: "Merhaba",
      country_id: 1,
      province_id: 34,
      district_id: 501,
      kvkk_consent: true,
      language: "tr",
      website: "",
    });
    const success = container.querySelector("[data-screen='success']");
    expect(success?.textContent).toContain(msg("success_title"));
    expect(container.querySelector("form")).toBeNull();
  });

  it("explains a 429 with the wait time", async () => {
    submitReply = {
      status: 429,
      body: { success: false, error: { code: "TOO_MANY_REQUESTS" } },
      headers: { "Retry-After": "1800" },
    };
    await render();
    await fillValid();
    await tickKvkk();
    await submit();
    const notice = container.querySelector("[data-notice='rate_limited']");
    expect(notice?.textContent).toContain(msg("errors.rate_limited"));
    expect(notice?.textContent).toContain(
      msg("errors.rate_limited_retry", { minutes: 30 }),
    );
    expect(container.querySelector("[data-screen='success']")).toBeNull();
  });

  it("marks the fields Go rejected", async () => {
    submitReply = {
      status: 400,
      body: {
        success: false,
        error: {
          code: "VALIDATION_ERROR",
          details: [{ field: "phone", message: "must be a valid phone" }],
        },
      },
    };
    await render();
    await fillValid();
    await tickKvkk();
    await submit();
    expect(field("phone").getAttribute("aria-invalid")).toBe("true");
    expect(container.textContent).toContain(msg("errors.phone_invalid"));
  });

  it("hides the honeypot from people", async () => {
    await render();
    const honeypot = field<HTMLInputElement>("website");
    expect(honeypot.tabIndex).toBe(-1);
    expect(honeypot.closest("[aria-hidden='true']")).not.toBeNull();
  });
});
