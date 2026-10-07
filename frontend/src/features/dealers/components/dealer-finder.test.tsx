// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mapProps = vi.hoisted(() => ({
  last: null as null | { center: unknown; markers: unknown[] },
}));
const session = vi.hoisted(() => ({ readOnly: false }));

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
vi.mock("@/components/common/leaflet-map", () => ({
  LeafletMap: (props: {
    center: unknown;
    markers: unknown[];
    testId: string;
  }) => {
    mapProps.last = props;
    return createElement("div", { "data-testid": props.testId });
  },
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "tr",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
  }),
}));

vi.mock("@/features/portal/lib/use-portal-read-only", () => ({
  usePortalReadOnly: () => session.readOnly,
}));

import { mapConfig } from "@/config/map";

import { DealerFinder } from "./dealer-finder";

const DEALER = {
  uuid: "11111111-1111-4111-8111-111111111111",
  slug: "kadikoy",
  name: "Olex Kadıköy",
  city: "İstanbul",
  district: "Kadıköy",
  latitude: 40.99,
  longitude: 29.03,
  distance_km: 3.456,
  accepts_appointments: true,
  whatsapp: "+905321234567",
};

let root: Root;
let host: HTMLDivElement;
let fetchMock: ReturnType<typeof vi.fn>;

function setGeolocation(value: unknown) {
  Object.defineProperty(navigator, "geolocation", {
    value,
    configurable: true,
  });
}

async function mount() {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root.render(createElement(DealerFinder));
  });
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await Promise.resolve();
    });
  }
}

function q(id: string) {
  return host.querySelector(`[data-testid="${id}"]`);
}

describe("DealerFinder", () => {
  beforeEach(() => {
    fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({ success: true, data: { items: [DEALER] } }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
    session.readOnly = false;
  });

  it("falls back to the default centre and warns when location is denied", async () => {
    setGeolocation({
      getCurrentPosition: (_ok: unknown, err: (e: { code: number }) => void) =>
        err({ code: 1 }),
    });
    await mount();

    const notice = q("dealer-location-notice");
    expect(notice?.getAttribute("data-reason")).toBe("denied");
    expect(notice?.textContent).toContain("portal.dealers.notice_denied");
    expect(mapProps.last?.center).toEqual(mapConfig.defaultCenter);

    const url = String(fetchMock.mock.calls[0]![0]);
    expect(url).toContain(`lat=${mapConfig.defaultCenter.lat.toFixed(6)}`);
    expect(url).toContain(`lng=${mapConfig.defaultCenter.lng.toFixed(6)}`);
    expect(url).toContain("radius_km=1000");
    expect(q("dealer-list")).not.toBeNull();
  });

  it("searches around the user and lists distance and WhatsApp", async () => {
    setGeolocation({
      getCurrentPosition: (
        ok: (p: { coords: { latitude: number; longitude: number } }) => void,
      ) => ok({ coords: { latitude: 41.0082, longitude: 28.9784 } }),
    });
    await mount();

    expect(q("dealer-location-notice")).toBeNull();
    const url = String(fetchMock.mock.calls[0]![0]);
    expect(url).toContain("lat=41.008200");
    expect(url).toContain("radius_km=100");

    expect(q("dealer-distance")?.textContent).toBe("3,5 km");
    expect(q("dealer-whatsapp")?.getAttribute("href")).toBe(
      "https://wa.me/905321234567",
    );
    expect(mapProps.last?.markers).toHaveLength(2);
  });

  it("hides WhatsApp for a dealer without an E.164 number", async () => {
    fetchMock.mockImplementation(
      async () =>
        new Response(
          JSON.stringify({
            success: true,
            data: { items: [{ ...DEALER, whatsapp: null }] },
          }),
          { status: 200 },
        ),
    );
    setGeolocation(undefined);
    await mount();
    expect(q("dealer-location-notice")?.getAttribute("data-reason")).toBe(
      "unsupported",
    );
    expect(q("dealer-item")).not.toBeNull();
    expect(q("dealer-whatsapp")).toBeNull();
  });

  it("shows an error with retry when the search fails", async () => {
    fetchMock.mockImplementation(
      async () => new Response("{}", { status: 500 }),
    );
    setGeolocation({
      getCurrentPosition: (_ok: unknown, err: (e: { code: number }) => void) =>
        err({ code: 2 }),
    });
    await mount();
    expect(q("dealer-error")?.textContent).toContain("portal.dealers.error");
  });

  it("offers booking at a dealer that takes portal appointments (TEC-327)", async () => {
    fetchMock.mockImplementation(
      async () =>
        new Response(
          JSON.stringify({
            success: true,
            data: {
              items: [
                DEALER,
                {
                  ...DEALER,
                  uuid: "22222222-2222-4222-8222-222222222222",
                  slug: "cankaya",
                  accepts_appointments: false,
                },
              ],
            },
          }),
          { status: 200 },
        ),
    );
    setGeolocation(undefined);
    await mount();
    const book = host.querySelectorAll('[data-testid="dealer-book"]');
    expect(book).toHaveLength(1);
    expect(book[0].getAttribute("href")).toBe(
      `/portal/appointments/new?dealer=${DEALER.uuid}&dealer_name=Olex+Kad%C4%B1k%C3%B6y`,
    );
  });

  it("hides booking from a read-only fleet session (TEC-327)", async () => {
    session.readOnly = true;
    setGeolocation(undefined);
    await mount();
    expect(q("dealer-item")).not.toBeNull();
    expect(q("dealer-book")).toBeNull();
  });
});
