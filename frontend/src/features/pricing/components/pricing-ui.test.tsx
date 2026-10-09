// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  click,
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

const state = vi.hoisted(() => ({
  publish: vi.fn(),
  getStatus: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    dir: "ltr",
    format: {
      number: (v: number | null) => (v === null ? "—" : String(v)),
      date: (v: string) => v,
      dateTime: (v: string) => v,
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
      percent: (v: number) => `${(v * 100).toFixed(2)}%`,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: state.toast }));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ isAuthenticated: true }),
}));
vi.mock("@/features/step-up-engine/services/stepup.service", () => ({
  stepUpService: { getStatus: state.getStatus },
}));
vi.mock("@/features/pricing/services/recommended.service", async (orig) => ({
  ...(await orig<object>()),
  recommendedService: { publish: state.publish },
}));

import { PriceTable } from "@/features/catalog/components/price-table";
import type { ProductPriceView } from "@/features/catalog/services/pricing.service";
import { StepUpProvider } from "@/features/step-up-engine";
import { DeviationBadge } from "./deviation-badge";
import { PublishDialog } from "./publish-dialog";

let m: Mounted;
beforeEach(() => {
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
  document.body.innerHTML = "";
});

const q = (sel: string) => document.body.querySelector(sel);

describe("DeviationBadge (TEC-507)", () => {
  it.each([
    ["20.00", 15, "true"],
    ["-15.00", 15, "true"],
    ["14.99", 15, "false"],
    ["-3.5", 15, "false"],
    ["16", 20, "false"],
  ])("deviation %s with threshold %d → over=%s", async (dev, th, over) => {
    await render(
      m,
      createElement(DeviationBadge, { value: dev, threshold: th }),
    );
    const badge = q('[data-testid="deviation-badge"]');
    expect(badge?.getAttribute("data-over-threshold")).toBe(over);
    // Over the threshold is the warning badge with its icon.
    expect(Boolean(badge?.querySelector("svg"))).toBe(over === "true");
  });

  it("renders nothing without a deviation", async () => {
    await render(
      m,
      createElement(DeviationBadge, { value: null, threshold: 15 }),
    );
    expect(q('[data-testid="deviation-badge"]')).toBeNull();
  });
});

const dealerView: ProductPriceView = {
  product_uuid: "p1",
  sku: "PPF-190",
  name: "Olex PPF 190",
  viewer: "dealer",
  prices: [
    {
      currency: "TRY",
      purchase_price: "800.00",
      purchase_price_source: "distributor",
      recommended: {
        price: "1000.00",
        currency: "TRY",
        country_iso2: "TR",
        scope: "country",
        effective_from: "2026-10-01",
      },
      deviation_pct: "25.00",
    },
  ],
};

describe("PriceTable recommended column (TEC-507)", () => {
  it("is not rendered without pricing.recommended.read", async () => {
    await render(
      m,
      createElement(PriceTable, { view: dealerView, showRecommended: false }),
    );
    expect(q('[data-testid="price-table"]')).not.toBeNull();
    expect(q('[data-testid="recommended-cell"]')).toBeNull();
    expect(document.body.textContent).not.toContain(
      "catalog.recommended.column",
    );
  });

  it("shows the recommended price and an over-threshold badge", async () => {
    await render(
      m,
      createElement(PriceTable, {
        view: dealerView,
        showRecommended: true,
        threshold: 15,
      }),
    );
    expect(q('[data-testid="recommended-cell"]')?.textContent).toContain(
      "1000.00 TRY",
    );
    expect(
      q('[data-testid="deviation-badge"]')?.getAttribute("data-over-threshold"),
    ).toBe("true");
  });

  it("never adds the column to the center's own view", async () => {
    await render(
      m,
      createElement(PriceTable, {
        view: {
          ...dealerView,
          viewer: "center",
          prices: [{ currency: "TRY", recommended_sale_price: "1000.00" }],
        },
        showRecommended: true,
      }),
    );
    expect(q('[data-testid="recommended-cell"]')).toBeNull();
  });
});

const basket = [
  {
    productUuid: "p1",
    productName: "Olex PPF 190",
    productSku: "PPF-190",
    country: "TR",
    currency: "TRY",
    price: "1100.00",
    previous: "1000.00",
  },
];

function renderPublish(onPublished = vi.fn()) {
  return render(
    m,
    createElement(
      StepUpProvider,
      null,
      createElement(PublishDialog, {
        open: true,
        rows: basket,
        today: "2026-10-09",
        onOpenChange: vi.fn(),
        onPublished,
      }),
    ),
  );
}

const picker = () => q("#recommended-effective-from");
/** Day cell of the open calendar (react-day-picker marks it data-day). */
const dayCell = (day: string) => q(`[data-day="${day}"]`);
const dayButton = (day: string) =>
  dayCell(day)?.querySelector("button") as HTMLButtonElement | null;

const submitButton = () =>
  q('[data-testid="publish-submit"]') as HTMLButtonElement | null;

describe("PublishDialog (TEC-507)", () => {
  beforeEach(() => {
    state.getStatus.mockResolvedValue({ valid: false, methods: ["password"] });
  });

  it("does not let a past effective day be chosen", async () => {
    await renderPublish();
    expect(submitButton()?.disabled).toBe(false);
    await click(picker());
    // Yesterday is disabled in the calendar, today and later are selectable.
    expect(dayCell("2026-10-08")?.getAttribute("data-disabled")).toBe("true");
    expect(dayButton("2026-10-08")?.disabled).toBe(true);
    expect(dayButton("2026-10-09")?.disabled).toBe(false);
    await click(dayButton("2026-10-08"));
    await click(submitButton());
    expect(state.publish).not.toHaveBeenCalled();
    expect(q('[data-testid="effective-from-error"]')).toBeNull();
  });

  it("opens the step-up dialog before publishing", async () => {
    await renderPublish();
    expect(document.body.textContent).not.toContain("stepup.dialog.title");
    await click(submitButton());
    await flush();
    expect(document.body.textContent).toContain("stepup.dialog.title");
    expect(state.publish).not.toHaveBeenCalled();
  });

  it("publishes the basket once the step-up is valid", async () => {
    state.getStatus.mockResolvedValue({ valid: true, methods: ["password"] });
    state.publish.mockResolvedValue({
      batch_id: "b1",
      price_count: 1,
      applied_count: 0,
      scheduled_count: 1,
      versions: [],
    });
    const onPublished = vi.fn();
    await renderPublish(onPublished);
    await click(picker());
    await click(dayButton("2026-10-20"));
    await click(submitButton());
    await flush();
    expect(state.publish).toHaveBeenCalledWith({
      effective_from: "2026-10-20",
      note: null,
      rows: [
        {
          product_uuid: "p1",
          country: "TR",
          currency: "TRY",
          price: "1100.00",
        },
      ],
    });
    expect(onPublished).toHaveBeenCalled();
  });
});
