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

import { registerMessages } from "@/lib/i18n/messages";
import ar from "@/locales/ar";
import tr from "@/locales/tr";

import { LandingView } from "./landing-view";

const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn(), prefetch: vi.fn() }),
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeAll(() => {
  registerMessages("tr", tr);
  registerMessages("ar", ar);
});

beforeEach(() => {
  push.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function render(locale: "en" | "tr" | "ar" = "en", lang?: string) {
  act(() => root.render(createElement(LandingView, { locale, lang })));
}

function typeInto(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function submit() {
  const form = container.querySelector<HTMLFormElement>(
    '[data-slot="warranty-lookup"]',
  );
  act(() => {
    form?.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
  });
}

describe("LandingView", () => {
  it("renders the protection solutions content and the entry links", () => {
    render();
    const main = container.querySelector('[data-slot="landing"]');
    expect(main?.getAttribute("dir")).toBe("ltr");
    expect(container.querySelector("h1")?.textContent).toContain(
      "Protection Solutions for Your Vehicle",
    );
    expect(container.querySelectorAll("[data-solution]")).toHaveLength(3);
    expect(
      container
        .querySelector('[data-slot="panel-login"]')
        ?.getAttribute("href"),
    ).toBe("/platform/login");
    expect(
      container
        .querySelector('[data-slot="portal-login"]')
        ?.getAttribute("href"),
    ).toBe("/portal/login");
    expect(
      container
        .querySelector('[data-slot="find-dealer"]')
        ?.getAttribute("href"),
    ).toBe("/portal/dealers");
    const logo = container.querySelector("img");
    expect(logo?.getAttribute("src")).toBe("/images/olex-logo-yatay.svg");
  });

  it("renders Turkish copy", () => {
    render("tr");
    expect(container.querySelector("h1")?.textContent).toContain(
      "Koruma Çözümleri",
    );
  });

  it("shows an error and does not navigate on an empty warranty number", () => {
    render();
    const input = container.querySelector<HTMLInputElement>(
      'input[name="warranty_no"]',
    )!;
    typeInto(input, "   ");
    submit();
    expect(push).not.toHaveBeenCalled();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      "Enter your warranty number.",
    );
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("navigates to /garanti/{no} with the trimmed, encoded number", () => {
    render();
    const input = container.querySelector<HTMLInputElement>(
      'input[name="warranty_no"]',
    )!;
    typeInto(input, "  AbC/12 ");
    submit();
    expect(push).toHaveBeenCalledWith("/garanti/AbC%2F12");
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it("keeps the ?lang= choice on the warranty link", () => {
    render("tr", "tr");
    const input = container.querySelector<HTMLInputElement>(
      'input[name="warranty_no"]',
    )!;
    typeInto(input, "XYZ");
    submit();
    expect(push).toHaveBeenCalledWith("/garanti/XYZ?lang=tr");
  });

  it("is right-to-left in Arabic", () => {
    render("ar");
    const main = container.querySelector('[data-slot="landing"]');
    expect(main?.getAttribute("dir")).toBe("rtl");
    expect(main?.getAttribute("lang")).toBe("ar");
    expect(container.querySelector("h1")?.textContent).toContain(
      "لحماية مركبتك",
    );
  });

  it("links the dealer application form only while it is open (TEC-320)", () => {
    render("tr", "tr");
    expect(
      container.querySelector('[data-slot="dealer-application"]'),
    ).toBeNull();
    act(() =>
      root.render(
        createElement(LandingView, {
          locale: "tr",
          lang: "tr",
          dealerApplicationOpen: true,
        }),
      ),
    );
    const link = container.querySelector('[data-slot="dealer-application"]');
    expect(link?.getAttribute("href")).toBe("/bayi-basvuru?lang=tr");
    expect(link?.textContent).toBe("Bayimiz olun");
  });
});
