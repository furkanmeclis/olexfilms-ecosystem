// @vitest-environment jsdom
import { Direction } from "radix-ui";
import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { AppLocale } from "@/config/i18n";
import { LocaleProvider, useLocale } from "@/providers/locale-provider";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type Probe = ReturnType<typeof useLocale> & { radixDir: string };

function ProbeComponent({ onRender }: { onRender: (p: Probe) => void }) {
  const ctx = useLocale();
  const radixDir = Direction.useDirection();
  useEffect(() => onRender({ ...ctx, radixDir }));
  return <span>{ctx.t("common.save")}</span>;
}

function clearCookies() {
  for (const part of document.cookie.split(";")) {
    const name = part.split("=")[0]?.trim();
    if (name) document.cookie = `${name}=; Max-Age=0; Path=/`;
  }
}

describe("LocaleProvider RTL", () => {
  let container: HTMLDivElement;
  let root: Root;
  let probe: Probe | null;
  const capture = (p: Probe) => {
    probe = p;
  };

  async function mount(initialLocale: AppLocale, initialTimeZone?: string) {
    await act(async () => {
      root.render(
        <LocaleProvider
          initialLocale={initialLocale}
          initialTimeZone={initialTimeZone}
        >
          <ProbeComponent onRender={capture} />
        </LocaleProvider>,
      );
    });
  }

  beforeEach(() => {
    clearCookies();
    window.localStorage.clear();
    document.documentElement.lang = "tr";
    document.documentElement.dir = "ltr";
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    probe = null;
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("switching to Arabic sets html[dir=rtl][lang=ar], Radix direction and the cookie", async () => {
    await mount("tr");
    expect(probe?.dir).toBe("ltr");
    expect(probe?.radixDir).toBe("ltr");

    await act(async () => {
      await probe!.setLocale("ar");
    });

    expect(document.documentElement.getAttribute("dir")).toBe("rtl");
    expect(document.documentElement.getAttribute("lang")).toBe("ar");
    expect(probe?.locale).toBe("ar");
    expect(probe?.dir).toBe("rtl");
    expect(probe?.radixDir).toBe("rtl");
    // The server reads this cookie on reload (see request-locale.test.ts).
    expect(document.cookie).toContain("NEXT_LOCALE=ar");
    // ar has no translations yet: en text, never a raw key.
    expect(container.textContent).toBe("Save");
  });

  it("starts RTL when the server resolved Arabic from the cookie", async () => {
    document.cookie = "NEXT_LOCALE=ar; Path=/";
    await mount("ar");
    expect(document.documentElement.getAttribute("dir")).toBe("rtl");
    expect(probe?.radixDir).toBe("rtl");
  });

  it("formats with the effective time zone", async () => {
    await mount("tr", "Europe/Istanbul");
    act(() => probe!.setTimeZone("Asia/Dubai"));
    expect(probe?.timeZone).toBe("Asia/Dubai");
    expect(probe?.format.time("2026-01-01T12:00:00Z")).toBe("16:00");
    expect(document.cookie).toContain("NEXT_TIMEZONE=Asia%2FDubai");
    // Invalid zones are ignored.
    act(() => probe!.setTimeZone("Nowhere/City"));
    expect(probe?.timeZone).toBe("Asia/Dubai");
  });
});
