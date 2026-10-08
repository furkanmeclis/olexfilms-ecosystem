// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShowcaseLeadForm } from "./showcase-lead-form";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

vi.stubGlobal("ResizeObserver", ResizeObserverMock);

const config = {
  dealer_code: "olex-kadikoy",
  dealer_name: "Olex Kadıköy",
  fields: ["name", "phone"],
  kvkk_text_version: 1,
  kvkk_text: "KVKK text",
  services: [
    {
      uuid: "11111111-1111-4111-8111-111111111111",
      kind: "custom",
      title: "Full body PPF",
    },
  ],
  whatsapp_chat_url: "https://wa.me/905321234567",
  form_token: "token",
  min_fill_seconds: 1,
  default_phone_country: "TR",
  preferred_locales: ["tr", "en"],
};

const messages: Record<string, string> = {
  "portal.dealer_page.form_title": "Request a quote",
  "portal.dealer_page.form_loading": "Loading",
  "portal.dealer_page.form_name": "Name",
  "portal.dealer_page.form_country": "Country",
  "portal.dealer_page.form_phone": "Phone",
  "portal.dealer_page.form_email": "Email",
  "portal.dealer_page.form_vehicle_brand": "Brand",
  "portal.dealer_page.form_vehicle_model": "Model",
  "portal.dealer_page.form_services": "Services",
  "portal.dealer_page.form_message": "Message",
  "portal.dealer_page.form_channel": "Channel",
  "portal.dealer_page.form_channel_whatsapp": "WhatsApp",
  "portal.dealer_page.form_channel_phone": "Phone",
  "portal.dealer_page.form_channel_email": "Email",
  "portal.dealer_page.form_honeypot": "Website",
  "portal.dealer_page.form_kvkk": "KVKK",
  "portal.dealer_page.form_submit": "Send request",
  "portal.dealer_page.form_submitting": "Sending",
  "portal.dealer_page.form_required": "Required",
  "portal.dealer_page.form_phone_invalid": "Invalid phone",
  "portal.dealer_page.form_email_invalid": "Invalid email",
  "portal.dealer_page.form_kvkk_required": "KVKK required",
  "portal.dealer_page.form_validation": "Validation",
  "portal.dealer_page.form_rate_limited": "Rate limited",
  "portal.dealer_page.form_failed": "Failed",
  "portal.dealer_page.form_success_title": "Request received",
  "portal.dealer_page.form_success_body": "Dealer will contact you.",
};

let host: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;

const t = (key: string, params?: Record<string, string | number>) => {
  let value = messages[key] ?? key;
  for (const [k, v] of Object.entries(params ?? {})) {
    value = value.replaceAll(`{{${k}}}`, String(v));
  }
  return value;
};

async function render() {
  await act(async () => {
    root.render(
      createElement(ShowcaseLeadForm, {
        code: "olex-kadikoy",
        locale: "en",
        t,
        fetchImpl: fetchMock,
      }),
    );
  });
  await act(async () => {
    await Promise.resolve();
  });
}

function field<T extends HTMLElement>(name: string) {
  return host.querySelector(`[name="${name}"]`) as T;
}

function type(name: string, value: string) {
  const el = field<HTMLInputElement>(name);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("ShowcaseLeadForm", () => {
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
    fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).includes("/lead-form/config")) {
        return new Response(JSON.stringify({ success: true, data: config }));
      }
      if (String(url).endsWith("/leads") && init?.method === "POST") {
        return new Response(
          JSON.stringify({ success: true, data: { received: true } }),
          {
            status: 202,
          },
        );
      }
      return new Response("{}", { status: 404 });
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("keeps submit disabled until KVKK and flags invalid phones", async () => {
    await render();
    const submit = host.querySelector(
      "button[type='submit']",
    ) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    await act(async () => {
      type("name", "Ayşe Yılmaz");
      type("phone", "12");
      field<HTMLButtonElement>("kvkk_consent").click();
    });
    expect(submit.disabled).toBe(false);

    await act(async () => submit.click());
    expect(host.textContent).toContain("Invalid phone");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("posts E.164 phone and shows success", async () => {
    await render();
    await act(async () => {
      type("name", "Ayşe Yılmaz");
      type("phone", "0532 123 45 67");
      field<HTMLButtonElement>("kvkk_consent").click();
    });
    await act(async () => {
      (
        host.querySelector("button[type='submit']") as HTMLButtonElement
      ).click();
    });
    const body = JSON.parse(String(fetchMock.mock.calls[1]![1]?.body));
    expect(body).toMatchObject({
      name: "Ayşe Yılmaz",
      phone: "+905321234567",
      kvkk_consent: true,
      language: "en",
      form_token: "token",
      website: "",
    });
    expect(
      host.querySelector('[data-screen="showcase-lead-success"]'),
    ).not.toBeNull();
  });
});
