import { describe, expect, it } from "vitest";

import en from "@/locales/en/notifications.json";
import tr from "@/locales/tr/notifications.json";

import {
  CHANNELS,
  DELIVERY_STATUSES,
  LANGUAGES,
  ROLES,
  deliveryStatusVariant,
} from "./options";

describe("notification center options", () => {
  it("covers the six channels and thirteen languages", () => {
    expect(CHANNELS).toHaveLength(6);
    expect(LANGUAGES).toHaveLength(13);
    expect(LANGUAGES).toContain("ar");
  });

  it("has a tr and en label for every option", () => {
    const keys = [
      ...CHANNELS.map((c) => `center.channels.${c}`),
      ...ROLES.map((r) => `center.roles.${r}`),
      ...DELIVERY_STATUSES.map((s) => `center.statuses.${s}`),
    ];
    for (const key of keys) {
      expect(en).toHaveProperty([key]);
      expect(tr).toHaveProperty([key]);
    }
  });

  it("maps statuses to badge variants", () => {
    expect(deliveryStatusVariant("sent")).toBe("default");
    expect(deliveryStatusVariant("failed")).toBe("danger");
    expect(deliveryStatusVariant("skipped_disabled")).toBe("outline");
  });
});
