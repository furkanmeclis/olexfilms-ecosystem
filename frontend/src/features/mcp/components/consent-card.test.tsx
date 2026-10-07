// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { OAuthConsent } from "@/features/mcp/services/mcp.service";

const calls = vi.hoisted(() => ({
  platform: [] as unknown[][],
  portal: [] as unknown[][],
  platformError: null as null | { status: number },
  portalError: null as null | { status: number },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: { number: (v: number) => String(v), dateTime: (v: string) => v },
  }),
}));
vi.mock("@/lib/api/platform-request", () => ({
  platformRequest: async (...args: unknown[]) => {
    calls.platform.push(args);
    if (calls.platformError) throw calls.platformError;
    return consentOf("dealer");
  },
}));
vi.mock("@/features/portal/lib/portal-client", () => ({
  portalRequest: async (...args: unknown[]) => {
    calls.portal.push(args);
    if (calls.portalError) throw calls.portalError;
    return consentOf("customer");
  },
}));

import { ConsentCard, type ConsentDecision } from "./consent-card";
import { loadConsent } from "./consent-screen";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function consentOf(realm: OAuthConsent["realm"]): OAuthConsent {
  const customer = realm === "customer";
  return {
    request_uuid: "00000000-0000-4000-8000-000000000001",
    client_id: "client-123",
    client_name: "Claude",
    redirect_host: "claude.ai",
    resource: customer ? "/mcp/customer" : "/mcp/dealer",
    resource_url: `https://olexfilms.test/mcp/${realm}`,
    realm,
    organizations: customer
      ? [{ uuid: "c-1", name: "Olex Center", type: "center", tool_count: 10 }]
      : [
          { uuid: "d-1", name: "Bayi Bir", type: "dealer", tool_count: 21 },
          { uuid: "d-2", name: "Bayi İki", type: "dealer", tool_count: 9 },
        ],
    expires_at: "2026-10-07T10:00:00Z",
  };
}

let container: HTMLDivElement;
let root: Root;
let decisions: ConsentDecision[];

beforeEach(() => {
  decisions = [];
  calls.platform = [];
  calls.portal = [];
  calls.platformError = null;
  calls.portalError = null;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(consent: OAuthConsent) {
  await act(async () => {
    root.render(
      createElement(ConsentCard, {
        consent,
        onDecide: (d: ConsentDecision) => decisions.push(d),
      }),
    );
  });
}

const byTestId = (id: string) =>
  container.querySelector<HTMLElement>(`[data-testid="${id}"]`);

async function click(el: Element | null) {
  expect(el).not.toBeNull();
  await act(async () => {
    (el as HTMLElement).click();
  });
}

describe("ConsentCard", () => {
  it("keeps Allow disabled until an organization is chosen", async () => {
    await render(consentOf("dealer"));
    const approve = byTestId("oauth-consent-approve") as HTMLButtonElement;
    expect(byTestId("oauth-consent-orgs")).not.toBeNull();
    expect(approve.disabled).toBe(true);
    expect(byTestId("oauth-consent-summary")).toBeNull();
    await click(approve);
    expect(decisions).toEqual([]);

    await click(container.querySelector('[role="radio"][value="d-2"]'));
    expect(approve.disabled).toBe(false);
    expect(byTestId("oauth-consent-summary")?.textContent).toContain(
      '"count":"9"',
    );
    await click(approve);
    expect(decisions).toEqual([
      { decision: "approve", organizationUuid: "d-2" },
    ]);
  });

  it("shows the redirect host warning and lets the user reject", async () => {
    await render(consentOf("user"));
    expect(byTestId("oauth-consent-redirect")?.textContent).toContain(
      "claude.ai",
    );
    await click(byTestId("oauth-consent-deny"));
    expect(decisions).toEqual([{ decision: "deny" }]);
  });

  it("has no organization picker on the customer endpoint", async () => {
    await render(consentOf("customer"));
    expect(byTestId("oauth-consent-orgs")).toBeNull();
    expect(container.querySelector('[role="radio"]')).toBeNull();
    const approve = byTestId("oauth-consent-approve") as HTMLButtonElement;
    expect(approve.disabled).toBe(false);
    expect(byTestId("oauth-consent-summary")?.textContent).toContain(
      '"count":"10"',
    );
    await click(approve);
    expect(decisions).toEqual([
      { decision: "approve", organizationUuid: null },
    ]);
  });

  it("cannot approve without a selectable organization", async () => {
    await render({ ...consentOf("dealer"), organizations: [] });
    expect(byTestId("oauth-consent-no-org")).not.toBeNull();
    expect(
      (byTestId("oauth-consent-approve") as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});

describe("loadConsent", () => {
  it("uses the panel session first and falls back to the portal on 403", async () => {
    calls.platformError = { status: 403 };
    const out = await loadConsent("req-1", { panel: true, portal: true });
    expect(out).toMatchObject({ kind: "ok", realm: "portal" });
    expect(calls.platform[0]).toEqual(["GET", "/v1/oauth/requests/req-1"]);
    expect(calls.portal[0]).toEqual(["portal/oauth/requests/req-1"]);
  });

  it("asks to sign in without a fitting session and reports a missing request", async () => {
    expect(await loadConsent("req-1", { panel: false, portal: false })).toEqual(
      { kind: "login" },
    );
    calls.platformError = { status: 403 };
    expect(await loadConsent("req-1", { panel: true, portal: false })).toEqual({
      kind: "login",
    });
    calls.platformError = { status: 404 };
    expect(await loadConsent("req-1", { panel: true, portal: true })).toEqual({
      kind: "missing",
    });
    expect(calls.portal).toEqual([]);
  });
});
