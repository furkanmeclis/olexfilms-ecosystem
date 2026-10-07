// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const form = vi.hoisted(() => ({ platformFormRequest: vi.fn() }));
const wizard = vi.hoisted(() => ({ getService: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer",
}));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: unknown;
  } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dateTime(${v})`,
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ slug: "org", type: state.orgType }),
}));
vi.mock("@/lib/api/platform-request", () => http);
vi.mock("@/lib/api/platform-form-request", () => form);
vi.mock("@/features/services/services/service-wizard.service", () => ({
  serviceWizardService: wizard,
  serviceWizardKeys: {
    service: (uuid: string) => ["service-wizard", "service", uuid],
  },
}));
// The SVG picker itself is covered by its own tests; here each available
// part is one toggle button.
vi.mock("@/features/services/components/car-part-picker", async () => {
  const { createElement: h } = await import("react");
  return {
    CarPartPicker: ({
      available,
      selected,
      onChange,
    }: {
      available: string[];
      selected: string[];
      onChange: (v: string[]) => void;
    }) =>
      h(
        "div",
        null,
        available.map((part) =>
          h(
            "button",
            {
              key: part,
              type: "button",
              "data-testid": `part-${part}`,
              onClick: () =>
                onChange(
                  selected.includes(part)
                    ? selected.filter((p) => p !== part)
                    : [...selected, part],
                ),
            },
            part,
          ),
        ),
      ),
  };
});

import { Permission } from "@/config/permissions";
import type { WarrantyClaim } from "@/features/warranty-claims/lib/claims";
import type { Warranty } from "@/features/warranty/lib/warranty-list";

import { OpenClaimDialog } from "./open-claim-section";
import { PortalClaimBadge } from "./portal-claim-badge";
import { WarrantyClaimDetailPage } from "./warranty-claim-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const claim: WarrantyClaim = {
  uuid: "claim-1",
  claim_no: 7,
  warranty_uuid: "w-1",
  status: "center_review",
  description: "Film peeling on the hood",
  coverage_check: {
    ok: true,
    reasons: [],
    checked_at: "2026-10-01T09:00:00Z",
  },
  parts: [
    { uuid: "cp-1", part_key: "body_kaput" } as NonNullable<
      WarrantyClaim["parts"]
    >[number],
  ],
  photos: [
    {
      uuid: "ph-1",
      created_at: "2026-10-01T10:00:00Z",
    } as NonNullable<WarrantyClaim["photos"]>[number],
  ],
  events: [
    {
      uuid: "ev-1",
      event_type: "created",
      created_at: "2026-10-01T09:00:00Z",
    },
  ],
  created_at: "2026-10-01T09:00:00Z",
  updated_at: "2026-10-02T09:00:00Z",
  organization_name: "Dealer A",
  warranty_no: "W-0001",
  product_name: "PPF Gloss",
};

const warranty = {
  uuid: "w-1",
  public_code: "PUB-1",
  status: "active",
  service: { uuid: "svc-1" },
} as unknown as Warranty;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  wizard.getService.mockResolvedValue({
    uuid: "svc-1",
    warranties: [{ uuid: "w-1", service_item_uuid: "item-1" }],
    items: [{ uuid: "item-1", applied_parts: ["body_kaput", "body_tavan"] }],
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
  state.orgType = "dealer";
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(el: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, el));
  });
  await flush();
}

const q = (id: string) =>
  document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function setText(el: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )!.set!;
  setter.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

function addFiles(input: HTMLInputElement, files: File[]) {
  Object.defineProperty(input, "files", { value: files, configurable: true });
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

describe("OpenClaimDialog", () => {
  it("keeps 'Submit for review' disabled until a photo is added", async () => {
    await render(
      createElement(OpenClaimDialog, {
        slug: "org",
        warranty,
        open: true,
        onOpenChange: () => {},
      }),
    );
    const submit = q("claim-open-submit") as HTMLButtonElement;
    expect(submit).not.toBeNull();
    expect(submit.disabled).toBe(true);

    // Only the warranted parts of the item are offered.
    expect(q("part-body_kaput")).not.toBeNull();
    expect(q("part-body_tavan")).not.toBeNull();
    await act(async () => q("part-body_kaput")!.click());
    await act(async () =>
      setText(
        q("claim-description") as HTMLTextAreaElement,
        "Film peeling on the hood",
      ),
    );
    // Parts and description but no photo: still disabled.
    expect(submit.disabled).toBe(true);

    // A too large photo is rejected and does not enable it.
    const big = new File(["x"], "big.jpg", { type: "image/jpeg" });
    Object.defineProperty(big, "size", { value: 13 * 1024 * 1024 });
    await act(async () =>
      addFiles(q("claim-photo-input") as HTMLInputElement, [big]),
    );
    expect(submit.disabled).toBe(true);
    expect(document.body.textContent).toContain(
      "warranty.claims.photos.too_large",
    );

    await act(async () =>
      addFiles(q("claim-photo-input") as HTMLInputElement, [
        new File(["x"], "a.jpg", { type: "image/jpeg" }),
        new File(["y"], "b.png", { type: "image/png" }),
      ]),
    );
    expect(q("claim-photo-list")?.querySelectorAll("li")).toHaveLength(2);
    expect(submit.disabled).toBe(false);
  });

  it("creates the claim, uploads the photos and submits it for review", async () => {
    http.platformRequest.mockImplementation((method: string, path: string) =>
      Promise.resolve(
        method === "POST" && path === "/v1/warranty-claims"
          ? { ...claim, status: "open" }
          : { ...claim, status: "dealer_review" },
      ),
    );
    form.platformFormRequest.mockResolvedValue({ uuid: "ph-2" });
    await render(
      createElement(OpenClaimDialog, {
        slug: "org",
        warranty,
        open: true,
        onOpenChange: () => {},
      }),
    );
    await act(async () => q("part-body_kaput")!.click());
    await act(async () =>
      setText(q("claim-description") as HTMLTextAreaElement, "Peeling"),
    );
    await act(async () =>
      addFiles(q("claim-photo-input") as HTMLInputElement, [
        new File(["x"], "a.jpg", { type: "image/jpeg" }),
      ]),
    );
    await act(async () => q("claim-open-submit")!.click());
    await flush();

    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/warranty-claims",
      {
        body: {
          warranty_uuid: "w-1",
          description: "Peeling",
          parts: [{ part_key: "body_kaput", service_item_uuid: "item-1" }],
        },
      },
    );
    expect(form.platformFormRequest).toHaveBeenCalledTimes(1);
    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/warranty-claims/claim-1/status",
      { body: { status: "dealer_review" } },
    );
  });
});

describe("WarrantyClaimDetailPage", () => {
  async function renderDetail(data: WarrantyClaim) {
    http.platformRequest.mockResolvedValue(data);
    await render(
      createElement(WarrantyClaimDetailPage, { slug: "org", uuid: data.uuid }),
    );
  }

  it("shows no approve / reject buttons to a dealer user", async () => {
    state.grants = new Set([
      Permission.WarrantyClaimsRead,
      Permission.WarrantyClaimsWrite,
    ]);
    state.orgType = "dealer";
    await renderDetail(claim);
    expect(q("claim-detail")).not.toBeNull();
    expect(q("claim-approve")).toBeNull();
    expect(q("claim-reject")).toBeNull();
    expect(q("claim-forward")).toBeNull();
  });

  it("shows approve / reject to the center on center_review", async () => {
    state.grants = new Set([
      Permission.WarrantyClaimsRead,
      Permission.WarrantyClaimsWrite,
      Permission.WarrantyClaimsReview,
      Permission.WarrantyClaimsDecide,
    ]);
    state.orgType = "center";
    await renderDetail(claim);
    expect(q("claim-approve")).not.toBeNull();
    expect(q("claim-reject")).not.toBeNull();
  });

  it("shows 'Forward to center' to the distributor on dealer_review", async () => {
    state.grants = new Set([
      Permission.WarrantyClaimsRead,
      Permission.WarrantyClaimsWrite,
      Permission.WarrantyClaimsReview,
    ]);
    state.orgType = "distributor";
    await renderDetail({ ...claim, status: "dealer_review" });
    expect(q("claim-forward")).not.toBeNull();
    expect(q("claim-approve")).toBeNull();
  });

  it("shows the warning band when coverage_check.ok is false", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await renderDetail({
      ...claim,
      coverage_check: {
        ok: false,
        reasons: ["warranty_period_expired"],
        checked_at: "2026-10-01T09:00:00Z",
      },
    });
    const band = q("coverage-warning");
    expect(band).not.toBeNull();
    expect(band!.textContent).toContain(
      "warranty.claims.coverage.reasons.warranty_period_expired",
    );
  });

  it("hides the warning band when coverage_check.ok is true", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await renderDetail(claim);
    expect(q("coverage-warning")).toBeNull();
    expect(q("claim-gallery")?.querySelectorAll("img")).toHaveLength(1);
    expect(q("claim-timeline")?.textContent).toContain(
      "warranty.claims.timeline.events.created",
    );
    // AI fields are empty in F3 and the API returned no cost summary.
    expect(q("claim-ai")).toBeNull();
    expect(q("claim-cost-summary")).toBeNull();
  });

  it("renders the cost summary and the re-application link when returned", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await renderDetail({
      ...claim,
      status: "reapplied",
      cost_summary: { product_cost: "100.00", labor: "50.00", currency: "TRY" },
      reapply_service: { uuid: "svc-2", service_no: "S-0002" },
    } as WarrantyClaim);
    expect(q("claim-cost-summary")?.textContent).toContain("150 TRY");
    expect(q("claim-reapply-link")?.getAttribute("href")).toBe(
      "/t/org/services/svc-2",
    );
  });
});

describe("PortalClaimBadge", () => {
  it("renders only the status and date, never description or photos", async () => {
    const rich = {
      uuid: "claim-1",
      warranty_uuid: "w-1",
      status: "dealer_review",
      created_at: "2026-10-01T09:00:00Z",
      updated_at: "2026-10-02T09:00:00Z",
      description: "SECRET DESCRIPTION",
      photos: [{ uuid: "ph-1", url: "https://example.test/p.jpg" }],
    } as unknown as Parameters<typeof PortalClaimBadge>[0]["claim"];
    await render(createElement(PortalClaimBadge, { claim: rich }));
    const badge = q("portal-claim-badge")!;
    expect(badge.textContent).toContain("warranty.claims.status.dealer_review");
    expect(badge.textContent).toContain("date(2026-10-02T09:00:00Z)");
    expect(container.textContent).not.toContain("SECRET DESCRIPTION");
    expect(container.querySelector("img")).toBeNull();
  });
});
