// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const photoApi = vi.hoisted(() => ({ intake: vi.fn(), uploadIntake: vi.fn() }));
const session = vi.hoisted(() => ({
  user: {
    isSuperAdmin: false,
    organizationRoles: ["dealer_staff"] as string[],
  },
  org: { uuid: "d1", type: "dealer" },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ user: session.user }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => session.org,
}));
vi.mock(
  "@/features/photo-standard/services/photo-standard.service",
  async (orig) => ({
    ...(await orig<object>()),
    photoStandardService: photoApi,
  }),
);

import { MAX_PHOTO_BYTES } from "@/features/photo-standard/lib/photo-standard";
import type { IntakePhotoList } from "@/features/photo-standard/services/photo-standard.service";

import { IntakePhotoGallery } from "./intake-photo-gallery";
import { IntakePhotosStep } from "./intake-photos-step";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  session.user = { isSuperAdmin: false, organizationRoles: ["dealer_staff"] };
  session.org = { uuid: "d1", type: "dealer" };
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);

const intake = (over: Partial<IntakePhotoList> = {}): IntakePhotoList => ({
  service_uuid: "s1",
  angles: [
    {
      angle: {
        uuid: "a1",
        key: "front",
        name: { tr: "Ön" },
        hint: { tr: "Önden çek" },
        example_url: "/v1/photo-standard/angles/a1/example",
        required: true,
        sort_order: 10,
        active: true,
      },
      missing: true,
      required: true,
    },
  ],
  missing: ["front"],
  ...over,
});

async function pick(input: Element | null, file: File) {
  if (!(input instanceof HTMLInputElement)) throw new Error("no input");
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

describe("IntakePhotosStep (TEC-500)", () => {
  const props = (data: IntakePhotoList) => ({
    serviceUuid: "s1",
    intake: { data, isLoading: false, isError: false, refetch: vi.fn() },
    flagged: [],
    onBack: vi.fn(),
    onNext: vi.fn(),
  });

  it("opens the rear camera and shows the example silhouette and hint", async () => {
    await render(createElement(IntakePhotosStep, props(intake())));
    const input = $('[data-testid="intake-input"]');
    expect(input?.getAttribute("accept")).toBe("image/*");
    expect(input?.getAttribute("capture")).toBe("environment");
    expect($('[data-testid="intake-example"]')?.getAttribute("src")).toContain(
      "/v1/photo-standard/angles/a1/example",
    );
    expect(container.textContent).toContain("Önden çek");
  });

  it("refuses a file over 12 MB before uploading", async () => {
    await render(createElement(IntakePhotosStep, props(intake())));
    const big = new File(["x"], "big.jpg", { type: "image/jpeg" });
    Object.defineProperty(big, "size", { value: MAX_PHOTO_BYTES + 1 });
    await pick($('[data-testid="intake-input"]'), big);
    expect(photoApi.uploadIntake).not.toHaveBeenCalled();
    expect($('[data-testid="intake-error"]')?.textContent).toBe(
      "photo_standard.intake.errors.too_large",
    );
  });

  it("uploads the original file with progress", async () => {
    photoApi.uploadIntake.mockImplementation(
      (_s: string, _k: string, _f: File, onProgress?: (p: number) => void) => {
        onProgress?.(40);
        return Promise.resolve({});
      },
    );
    await render(createElement(IntakePhotosStep, props(intake())));
    const ok = new File(["jpeg"], "ok.jpg", { type: "image/jpeg" });
    await pick($('[data-testid="intake-input"]'), ok);
    expect(photoApi.uploadIntake).toHaveBeenCalledWith(
      "s1",
      "front",
      ok,
      expect.any(Function),
    );
    expect($('[data-testid="intake-error"]')).toBeNull();
  });

  it("disables İleri with the reason while a required angle is missing", async () => {
    await render(createElement(IntakePhotosStep, props(intake())));
    const next = $('[data-testid="photos-next"]') as HTMLButtonElement;
    expect(next.disabled).toBe(true);
    expect(next.title).toBe("photo_standard.intake.next_blocked");

    act(() => root.unmount());
    root = createRoot(container);
    await render(
      createElement(IntakePhotosStep, props(intake({ missing: [] }))),
    );
    expect(
      ($('[data-testid="photos-next"]') as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});

describe("IntakePhotoGallery (TEC-500)", () => {
  const withPhoto = (): IntakePhotoList =>
    intake({
      missing: [],
      angles: [
        {
          ...intake().angles[0],
          missing: false,
          photo: {
            uuid: "p1",
            angle_key: "front",
            url: "/v1/services/s1/intake-photos/front/file",
            mime: "image/jpeg",
            size: 10,
            sha256: "0".repeat(64),
            exif_taken_at: "2026-10-01T09:00:00Z",
            exif_lat: "41.0082",
            exif_lng: "28.9784",
            created_at: "2026-10-07T10:00:00Z",
          },
        },
      ],
    });

  const renderGallery = () =>
    render(
      createElement(IntakePhotoGallery, {
        slug: "acme",
        serviceUuid: "s1",
        serviceOrgUuid: "d1",
      }),
    );

  it("hides the location from staff even when the payload has it", async () => {
    photoApi.intake.mockResolvedValue(withPhoto());
    await renderGallery();
    expect($('[data-testid="intake-gallery-item"]')).not.toBeNull();
    expect(container.textContent).toContain(
      'photo_standard.gallery.taken_exif {"date":"2026-10-01T09:00:00Z"}',
    );
    expect($('[data-testid="intake-location"]')).toBeNull();
  });

  it("shows a map link to the service dealer's owner", async () => {
    session.user = { isSuperAdmin: false, organizationRoles: ["dealer_owner"] };
    photoApi.intake.mockResolvedValue(withPhoto());
    await renderGallery();
    expect(
      $('[data-testid="intake-location"]')?.getAttribute("href"),
    ).toContain("mlat=41.0082");
  });
});
