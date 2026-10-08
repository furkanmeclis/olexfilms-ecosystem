// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Showcase } from "@/features/showcase/services/showcase.service";

const mocks = vi.hoisted(() => ({
  features: vi.fn(),
  get: vi.fn(),
  reviewList: vi.fn(),
  platformGet: vi.fn(),
  review: vi.fn(),
  tables: [] as unknown[],
}));

vi.mock("next/navigation", () => ({
  useParams: () => ({ slug: "dealer-a" }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "tr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key}:${Object.values(params).join(",")}` : key,
    format: { dateTime: (value: string) => value },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "dealer-a",
    type: "dealer",
  }),
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    list: mocks.features,
    request: vi.fn(),
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({
    children,
    actions,
  }: {
    children: ReactNode;
    actions?: ReactNode;
  }) => (
    <main>
      <div data-testid="actions">{actions}</div>
      {children}
    </main>
  ),
  EntityToolbar: () => null,
  EntityTable: (props: {
    data: { organization: { uuid: string; name: string } }[];
    onRowClick?: (row: {
      organization: { uuid: string; name: string };
    }) => void;
  }) => {
    mocks.tables.push(props);
    return (
      <button type="button" onClick={() => props.onRowClick?.(props.data[0])}>
        open-row
      </button>
    );
  },
}));
vi.mock("@/features/showcase/services/showcase.service", async (orig) => {
  const actual = await orig<object>();
  return {
    ...actual,
    showcaseService: {
      get: mocks.get,
      save: vi.fn(),
      submit: vi.fn(),
      setGoogleRating: vi.fn(),
      createService: vi.fn(),
      deleteService: vi.fn(),
      reorderServices: vi.fn(),
      uploadPhoto: vi.fn(),
      deletePhoto: vi.fn(),
      reorderPhotos: vi.fn(),
      photoUrl: (uuid: string) => `/photo/${uuid}`,
      reviewList: mocks.reviewList,
      platformGet: mocks.platformGet,
      review: mocks.review,
    },
    showcaseKeys: {
      all: ["showcase"],
      editor: (org?: string) => ["showcase", "editor", org ?? "self"],
      review: (params: unknown) => ["showcase", "review", params],
      platform: (org: string) => ["showcase", "platform", org],
    },
  };
});

import { PlatformShowcasesPage } from "./platform-showcases-page";
import { ShowcaseEditorPage } from "./showcase-editor-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let container: HTMLDivElement;

function sampleShowcase(overrides: Partial<Showcase> = {}): Showcase {
  return {
    uuid: "showcase-1",
    organization: {
      uuid: "org-1",
      code: "dealer-a",
      name: "Dealer A",
      type: "dealer",
      city: "Istanbul",
    },
    status: "draft",
    content: { tr: { headline: "Başlık", about: "Hakkında" } },
    working_hours: {},
    social_links: {},
    seo_keywords: [],
    google_place_id: null,
    google_rating: null,
    google_review_count: null,
    google_rating_source: null,
    google_rating_updated_at: null,
    published_content: null,
    published_at: null,
    submitted_at: null,
    reviewed_at: null,
    review_note: null,
    updated_at: "2026-10-08T10:00:00Z",
    approval_required: true,
    max_photos: 12,
    services: [],
    photos: [],
    places_configured: false,
    manual_rating_allowed: true,
    google_place_id_suggestion: null,
    ...overrides,
  };
}

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client} key={crypto.randomUUID()}>
        {node}
      </QueryClientProvider>,
    );
  });
  for (let i = 0; i < 4; i += 1) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  mocks.features.mockResolvedValue({ enabled: ["dealer_showcase"], items: [] });
  mocks.get.mockResolvedValue(sampleShowcase());
  mocks.reviewList.mockResolvedValue({
    items: [
      {
        uuid: "row-1",
        organization: {
          uuid: "org-1",
          code: "dealer-a",
          name: "Dealer A",
          type: "dealer",
          city: "Istanbul",
        },
        status: "pending_review",
        submitted_at: null,
        published_at: null,
        reviewed_at: null,
        review_note: null,
        updated_at: "2026-10-08T10:00:00Z",
      },
    ],
    total: 1,
    limit: 20,
    offset: 0,
  });
  mocks.platformGet.mockResolvedValue(
    sampleShowcase({ status: "pending_review" }),
  );
  mocks.review.mockResolvedValue(sampleShowcase({ status: "published" }));
  mocks.tables = [];
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

describe("Showcase editor", () => {
  it("uses review submit text when approval is on and publish text when it is off", async () => {
    await render(<ShowcaseEditorPage />);
    expect(document.body.textContent).toContain(
      "showcase.actions.submit_review",
    );

    mocks.get.mockResolvedValue(sampleShowcase({ approval_required: false }));
    await render(<ShowcaseEditorPage />);
    expect(document.body.textContent).toContain("showcase.actions.publish");
  });

  it("disables photo upload after the max photo count and rejects files over 5 MB", async () => {
    mocks.get.mockResolvedValue(
      sampleShowcase({
        photos: Array.from({ length: 12 }, (_, i) => ({
          uuid: `photo-${i}`,
          mime: "image/png" as const,
          size_bytes: 100,
          caption: {},
          sort_order: i,
          created_at: "2026-10-08T10:00:00Z",
        })),
      }),
    );
    await render(<ShowcaseEditorPage />);
    expect(
      container.querySelector<HTMLInputElement>('input[type="file"]')?.disabled,
    ).toBe(true);
    expect(document.body.textContent).toContain(
      "showcase.gallery.limit_reached",
    );

    mocks.get.mockResolvedValue(sampleShowcase());
    await render(<ShowcaseEditorPage />);
    const input =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    const large = new File([new Uint8Array(5 * 1024 * 1024 + 1)], "big.png", {
      type: "image/png",
    });
    await act(async () => {
      Object.defineProperty(input, "files", {
        value: [large],
        configurable: true,
      });
      input?.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(document.body.textContent).toContain("showcase.gallery.too_large");
  });

  it("renders FeatureDisabled instead of the editor when the module is off", async () => {
    mocks.features.mockResolvedValue({ enabled: [], items: [] });
    await render(<ShowcaseEditorPage />);
    expect(
      container.querySelector('[data-testid="feature-disabled"]'),
    ).not.toBeNull();
    expect(mocks.get).not.toHaveBeenCalled();
  });
});

describe("Platform showcase queue", () => {
  it("requires a note before rejecting a showcase", async () => {
    await render(<PlatformShowcasesPage />);
    await act(async () => {
      container.querySelector("button")?.click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const reject = [...document.body.querySelectorAll("button")].find(
      (button) => button.textContent?.includes("showcase.queue.reject"),
    );
    await act(async () => {
      reject?.click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(document.body.textContent).toContain(
      "showcase.queue.reject_note_required",
    );
    expect(mocks.review).not.toHaveBeenCalled();
  });
});
