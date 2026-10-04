// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const reviewApi = vi.hoisted(() => ({ get: vi.fn(), create: vi.fn() }));
const readOnly = vi.hoisted(() => ({ value: false }));

vi.mock("@/features/portal/lib/use-portal-read-only", () => ({
  usePortalReadOnly: () => readOnly.value,
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock(
  "@/features/portal/services/portal-service-review.service",
  async (orig) => ({
    ...(await orig<object>()),
    portalServiceReviewApi: reviewApi,
  }),
);

import { PortalApiError } from "@/features/portal/lib/portal-client";
import {
  googleReviewUrl,
  type PortalServiceReviewState,
} from "@/features/portal/services/portal-service-review.service";

import { PortalServiceReview } from "./portal-service-review";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const SERVICE = "11111111-1111-4111-8111-111111111111";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  readOnly.value = false;
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(PortalServiceReview, { serviceUuid: SERVICE }),
      ),
    );
  });
  await flush();
}

function state(
  patch: Partial<PortalServiceReviewState> = {},
): PortalServiceReviewState {
  return {
    review: null,
    can_review: true,
    google_business_url: null,
    questions: [],
    products: [],
    ...patch,
  };
}

const q = <T extends Element = HTMLElement>(sel: string) =>
  container.querySelector<T & Element>(sel);

async function click(el: Element | null) {
  expect(el).not.toBeNull();
  await act(async () => {
    (el as HTMLElement).click();
  });
}

function setComment(value: string) {
  const ta = q<HTMLTextAreaElement>("textarea")!;
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )!.set!;
  act(() => {
    setter.call(ta, value);
    ta.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function submit() {
  const form = q<HTMLFormElement>("[data-testid=portal-review-form]")!;
  await act(async () => {
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
  });
  await flush();
}

describe("PortalServiceReview (TEC-244)", () => {
  it("submits both ratings and the trimmed comment, then shows the review", async () => {
    reviewApi.get.mockResolvedValue(state());
    reviewApi.create.mockResolvedValue(
      state({
        can_review: false,
        review: {
          uuid: "r1",
          platform_rating: 4,
          product_rating: 5,
          comment: "Great",
          is_anonymous: false,
          source: "portal",
          answers: [],
          created_at: "2026-10-03T10:00:00Z",
        },
      }),
    );
    await render();
    expect(reviewApi.get).toHaveBeenCalledWith(SERVICE);
    expect(q("[data-testid=portal-review-form]")).not.toBeNull();

    await click(q("[data-rating=platform] [data-star='4']"));
    await click(q("[data-rating=product] [data-star='5']"));
    setComment("  Great  ");
    await submit();

    expect(reviewApi.create).toHaveBeenCalledWith(SERVICE, {
      platform_rating: 4,
      product_rating: 5,
      comment: "Great",
      is_anonymous: false,
      source: "portal",
    });
    expect(reviewApi.create.mock.calls[0]?.[1]).not.toHaveProperty("answers");
    expect(q("[data-testid=portal-review-form]")).toBeNull();
    expect(q("[data-testid=portal-review-done]")?.textContent).toContain(
      "Great",
    );
  });

  it("does not submit without both ratings", async () => {
    reviewApi.get.mockResolvedValue(state());
    await render();
    await click(q("[data-rating=platform] [data-star='3']"));
    await submit();
    expect(reviewApi.create).not.toHaveBeenCalled();
    expect(q("[role=alert]")?.textContent).toBe(
      "portal.review.rating_required",
    );
  });

  it("shows the already-reviewed message on 409", async () => {
    reviewApi.get.mockResolvedValue(state());
    reviewApi.create.mockRejectedValue(
      new PortalApiError("dup", 409, "SERVICE_ALREADY_REVIEWED", []),
    );
    await render();
    await click(q("[data-rating=platform] [data-star='2']"));
    await click(q("[data-rating=product] [data-star='2']"));
    await submit();
    expect(reviewApi.create).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("portal.review.already");
  });

  it("shows the Google button only when the dealer has a URL", async () => {
    reviewApi.get.mockResolvedValue(state());
    await render();
    expect(q("[data-testid=review-google]")).toBeNull();

    act(() => root.unmount());
    root = createRoot(container);
    reviewApi.get.mockResolvedValue(
      state({ google_business_url: "https://g.page/r/dealer" }),
    );
    await render();
    const link = q<HTMLAnchorElement>("[data-testid=review-google]");
    expect(link?.getAttribute("href")).toBe("https://g.page/r/dealer");
    expect(link?.getAttribute("target")).toBe("_blank");
  });

  it("hides the form from a read-only fleet session (TEC-245)", async () => {
    readOnly.value = true;
    reviewApi.get.mockResolvedValue(state());
    await render();
    expect(q("[data-testid=portal-service-review]")).toBeNull();

    act(() => root.unmount());
    root = createRoot(container);
    reviewApi.get.mockResolvedValue(
      state({ google_business_url: "https://g.page/r/dealer" }),
    );
    await render();
    expect(q("[data-testid=portal-review-form]")).toBeNull();
    expect(q("[data-testid=review-google]")).not.toBeNull();
    expect(container.textContent).not.toContain("portal.review.not_completed");
  });

  it("shows the not-completed note when the service cannot be reviewed", async () => {
    reviewApi.get.mockResolvedValue(state({ can_review: false }));
    await render();
    expect(q("[data-testid=portal-review-form]")).toBeNull();
    expect(container.textContent).toContain("portal.review.not_completed");
  });
});

describe("googleReviewUrl", () => {
  it("accepts only https links", () => {
    expect(googleReviewUrl(null)).toBeNull();
    expect(googleReviewUrl("  ")).toBeNull();
    expect(googleReviewUrl("javascript:alert(1)")).toBeNull();
    expect(googleReviewUrl(" https://g.page/r/x ")).toBe("https://g.page/r/x");
  });
});
