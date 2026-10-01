// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { pendingConsents, decideConsent } = vi.hoisted(() => ({
  pendingConsents: vi.fn(),
  decideConsent: vi.fn(async (..._args: unknown[]) => ({})),
}));

vi.mock("@/features/portal/lib/portal-client", () => ({
  portalApi: { pendingConsents, decideConsent },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (k: string) => k, locale: "tr" }),
}));

import { AIConsentDialog } from "./ai-consent-dialog";

const TEXT = {
  uuid: "u1",
  kind: "ai_guidelines",
  locale: "tr",
  version: 3,
  body: "## Yönerge\n\n- madde",
};

let root: Root;
let host: HTMLDivElement;

async function mount() {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root.render(createElement(AIConsentDialog));
  });
  // Let the pending request resolve.
  await act(async () => {
    await Promise.resolve();
  });
}

function dialog() {
  return document.querySelector('[data-testid="ai-consent-dialog"]');
}

describe("AI consent dialog (K22)", () => {
  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    pendingConsents.mockReset();
    decideConsent.mockClear();
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = "";
  });

  it("shows on first sign-in with the box unchecked; continue records a decline", async () => {
    pendingConsents.mockResolvedValue({ items: [TEXT] });
    await mount();

    expect(dialog()).not.toBeNull();
    const box = document.getElementById("ai-consent-accept");
    expect(box?.getAttribute("aria-checked")).toBe("false");
    expect(dialog()?.textContent).toContain("Yönerge");

    const button = Array.from(document.querySelectorAll("button")).find(
      (b) => b.textContent === "portal.consent.continue",
    );
    await act(async () => {
      button?.click();
    });
    expect(decideConsent).toHaveBeenCalledWith(TEXT, false);
    expect(dialog()).toBeNull();
  });

  it("records an accept when the box is ticked", async () => {
    pendingConsents.mockResolvedValue({ items: [TEXT] });
    await mount();
    const box = document.getElementById("ai-consent-accept");
    await act(async () => {
      box?.click();
    });
    expect(box?.getAttribute("aria-checked")).toBe("true");
    const button = Array.from(document.querySelectorAll("button")).find(
      (b) => b.textContent === "portal.consent.continue",
    );
    await act(async () => {
      button?.click();
    });
    expect(decideConsent).toHaveBeenCalledWith(TEXT, true);
  });

  it("does not show when nothing is pending (second sign-in)", async () => {
    pendingConsents.mockResolvedValue({ items: [] });
    await mount();
    expect(dialog()).toBeNull();
    expect(decideConsent).not.toHaveBeenCalled();
  });
});
