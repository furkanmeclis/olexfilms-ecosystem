// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/common/leaflet-map", () => ({
  LeafletMap: (props: {
    testId: string;
    onMapClick?: (p: { lat: number; lng: number }) => void;
  }) =>
    createElement("button", {
      type: "button",
      "data-testid": props.testId,
      onClick: () => props.onMapClick?.({ lat: 41.00823456, lng: 28.9784 }),
    }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ locale: "en", dir: "ltr", t: (key: string) => key }),
}));

import { SettingsForm } from "@/features/io/components/settings-form";
import {
  COORDINATE_ERRORS,
  settingsFormSchema,
  toPatchPayload,
  toSettingsFormValues,
} from "@/features/io/schemas/settings-form";
import type { AppSettings } from "@/features/io/types";

const SETTINGS: AppSettings = {
  company_name: "Olex Kadıköy",
  tagline: "",
  primary_color: "#112233",
  address: "",
  city: "İstanbul",
  district: "Kadıköy",
  phone: "",
  email: "",
  website: "",
  footer_text: "",
  paper_size: "A4",
  logo_url: null,
  latitude: null,
  longitude: null,
};

const base = toSettingsFormValues(SETTINGS, "#000000");

function issues(latitude: string, longitude: string) {
  const r = settingsFormSchema.safeParse({ ...base, latitude, longitude });
  return r.success
    ? []
    : r.error.issues.map((i) => `${i.path.join(".")}:${i.message}`);
}

describe("settings coordinates schema", () => {
  it("accepts both empty or both valid", () => {
    expect(issues("", "")).toEqual([]);
    expect(issues("41.0082", "28.9784")).toEqual([]);
    expect(issues("-90", "180")).toEqual([]);
    expect(issues("41,0082", "28,9784")).toEqual([]);
  });

  it("rejects out of range and non-numeric values", () => {
    expect(issues("91", "28")).toEqual([
      `latitude:${COORDINATE_ERRORS.latitude}`,
    ]);
    expect(issues("41", "-180.5")).toEqual([
      `longitude:${COORDINATE_ERRORS.longitude}`,
    ]);
    expect(issues("abc", "28")).toEqual([
      `latitude:${COORDINATE_ERRORS.latitude}`,
    ]);
  });

  it("requires the pair", () => {
    expect(issues("41", "")).toEqual([`longitude:${COORDINATE_ERRORS.pair}`]);
    expect(issues("", "28")).toEqual([`latitude:${COORDINATE_ERRORS.pair}`]);
  });

  it("sends numbers or nulls for the tenant only", () => {
    const v = { ...base, latitude: "41.00823456", longitude: "28,9784" };
    expect(toPatchPayload(v, { withCoordinates: true })).toMatchObject({
      latitude: 41.008235,
      longitude: 28.9784,
    });
    expect(toPatchPayload(base, { withCoordinates: true })).toMatchObject({
      latitude: null,
      longitude: null,
    });
    expect(toPatchPayload(v)).not.toHaveProperty("latitude");
  });

  it("fills the form from saved coordinates", () => {
    const v = toSettingsFormValues(
      { ...SETTINGS, latitude: 41.0082, longitude: 28.9784 },
      "#000000",
    );
    expect(v.latitude).toBe("41.0082");
    expect(v.longitude).toBe("28.9784");
  });
});

// The form layout and Radix controls measure themselves.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

let root: Root;
let host: HTMLDivElement;

async function mount(onSubmit = vi.fn(async () => {})) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root.render(
      createElement(SettingsForm, {
        settings: SETTINGS,
        canWrite: true,
        isSaving: false,
        isLogoUploading: false,
        isLogoRemoving: false,
        showLocation: true,
        onSubmit,
        onUploadLogo: () => {},
        onRemoveLogo: () => {},
      }),
    );
  });
  return onSubmit;
}

function input(name: string) {
  return host.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
}

async function type(name: string, value: string) {
  const el = input(name);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function submit() {
  await act(async () => {
    host
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

describe("tenant settings form coordinates (panel)", () => {
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("shows an error for an invalid coordinate and does not save", async () => {
    const onSubmit = await mount();
    expect(
      host.querySelector('[data-testid="coordinates-field"]'),
    ).not.toBeNull();
    await type("latitude", "123");
    await type("longitude", "28.97");
    await submit();
    expect(host.textContent).toContain(COORDINATE_ERRORS.latitude);
    expect(input("latitude").getAttribute("aria-invalid")).toBe("true");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows the pair error when only one value is given", async () => {
    const onSubmit = await mount();
    await type("latitude", "41");
    await submit();
    expect(host.textContent).toContain(COORDINATE_ERRORS.pair);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("fills both fields from a map tap and saves them", async () => {
    const onSubmit = await mount();
    await act(async () => {
      host
        .querySelector<HTMLButtonElement>('[data-testid="coordinates-map"]')!
        .click();
    });
    expect(input("latitude").value).toBe("41.008235");
    expect(input("longitude").value).toBe("28.9784");
    await submit();
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ latitude: 41.008235, longitude: 28.9784 }),
    );
  });
});
