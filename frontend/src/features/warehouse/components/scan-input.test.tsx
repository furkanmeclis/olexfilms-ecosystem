// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));

import { ScanInput } from "./scan-input";
import {
  fill,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "./test-helpers";

let m: Mounted;
beforeEach(() => {
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const input = () =>
  m.container.querySelector<HTMLInputElement>('[data-testid="scan-input"]');
const form = () => m.container.querySelector("form");

describe("ScanInput (TEC-231)", () => {
  it("focuses on mount and submits the normalized code on Enter", async () => {
    const onScan = vi.fn();
    await render(m, createElement(ScanInput, { onScan }));
    expect(document.activeElement).toBe(input());

    // A scanner may append a CR or a tab; a lower-case QR prefix is fixed.
    await fill(input(), "  ofw:loc:WH1-A-01\r");
    await submit(form());
    expect(onScan).toHaveBeenCalledTimes(1);
    expect(onScan).toHaveBeenCalledWith("OFW:LOC:WH1-A-01");
    // Cleared and still focused for the next scan.
    expect(input()?.value).toBe("");
    expect(document.activeElement).toBe(input());

    await fill(input(), "OLEX-00000123");
    await submit(form());
    expect(onScan).toHaveBeenLastCalledWith("OLEX-00000123");
  });

  it("ignores an empty scan and says so", async () => {
    const onScan = vi.fn();
    await render(m, createElement(ScanInput, { onScan }));
    await fill(input(), "   \t");
    await submit(form());
    expect(onScan).not.toHaveBeenCalled();
    expect(input()?.getAttribute("aria-invalid")).toBe("true");
    expect(m.container.querySelector('[role="alert"]')?.textContent).toBe(
      "warehouse.scan.empty",
    );

    await fill(input(), "A");
    expect(m.container.querySelector('[role="alert"]')).toBeNull();
  });

  it("does not submit while busy and uses its own id prefix", async () => {
    const onScan = vi.fn();
    await render(
      m,
      createElement(ScanInput, { onScan, busy: true, id: "entry-barcode" }),
    );
    const el = m.container.querySelector<HTMLInputElement>(
      '[data-testid="entry-barcode-input"]',
    );
    expect(
      m.container
        .querySelector('[data-testid="entry-barcode-submit"]')
        ?.hasAttribute("disabled"),
    ).toBe(true);
    await fill(el, "OLEX-1");
    await submit(form());
    expect(onScan).not.toHaveBeenCalled();
    expect(el?.value).toBe("OLEX-1");
  });

  it("does not steal the focus when autoFocus is off", async () => {
    await render(
      m,
      createElement(ScanInput, { onScan: vi.fn(), autoFocus: false }),
    );
    expect(document.activeElement).not.toBe(input());
  });
});
