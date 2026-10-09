import { describe, expect, it } from "vitest";

import {
  MODULE_REQUEST_NOTE_MAX,
  modulePriceKind,
  modulePriceText,
  moduleRequestAction,
  requestNoteError,
} from "./module-meta";

const t = (key: string, params?: Record<string, string | number>) =>
  params ? `${key}:${JSON.stringify(params)}` : key;
const currency = (amount: number, code: string) =>
  `${amount.toFixed(2)} ${code}`;

const free = {
  free_default: true,
  paid: false,
  price: null,
  contact_for_price: false,
};
const priced = {
  free_default: false,
  paid: true,
  price: {
    amount: "99.50",
    currency: "TRY",
    recurrence: "monthly" as const,
    item_uuid: "i",
    item_name: "Paket",
  },
  contact_for_price: false,
};
const contact = {
  free_default: false,
  paid: true,
  price: null,
  contact_for_price: true,
};

describe("modulePriceText (TEC-509)", () => {
  it("on by default: free, no service record needed", () => {
    expect(modulePriceKind(free)).toBe("free_default");
    expect(modulePriceText(t, currency, free)).toBe(
      "modules.price.free_default",
    );
  });

  it("module bundle on sale: paid with price and period", () => {
    expect(modulePriceKind(priced)).toBe("paid");
    expect(modulePriceText(t, currency, priced)).toBe(
      'modules.price.paid_monthly:{"price":"99.50 TRY"}',
    );
    expect(
      modulePriceText(t, currency, {
        ...priced,
        price: { ...priced.price, recurrence: "yearly" },
      }),
    ).toBe('modules.price.paid_yearly:{"price":"99.50 TRY"}');
  });

  it("paid with nothing on sale: contact us for the price", () => {
    expect(modulePriceKind(contact)).toBe("contact");
    expect(modulePriceText(t, currency, contact)).toBe("modules.price.contact");
  });

  it("a free add-on that is not on by default is plain free", () => {
    expect(
      modulePriceKind({ ...contact, paid: false, contact_for_price: false }),
    ).toBe("free");
  });
});

describe("moduleRequestAction (TEC-509)", () => {
  const base = {
    level: "addon" as const,
    enabled: false,
    upstream_enabled: true,
  };

  it("core modules offer neither request nor close", () => {
    expect(moduleRequestAction({ ...base, level: "core", enabled: true })).toBe(
      "none",
    );
    expect(moduleRequestAction({ ...base, level: "core" })).toBe("none");
  });

  it("an add-on off at the level above has no request", () => {
    expect(moduleRequestAction({ ...base, upstream_enabled: false })).toBe(
      "upstream_closed",
    );
  });

  it("open modules need nothing; closed ones can be requested", () => {
    expect(moduleRequestAction({ ...base, enabled: true })).toBe("none");
    expect(moduleRequestAction(base)).toBe("request");
  });

  it("a pending request can be withdrawn; a rejected one asked again", () => {
    const request = {
      uuid: "r",
      note: "",
      decision_note: "",
      created_at: "2026-10-01T00:00:00Z",
      decided_at: null,
    };
    expect(
      moduleRequestAction({
        ...base,
        request: { ...request, status: "pending" },
      }),
    ).toBe("pending");
    expect(
      moduleRequestAction({
        ...base,
        request: { ...request, status: "rejected" },
      }),
    ).toBe("request");
  });
});

describe("requestNoteError (TEC-509)", () => {
  it("accepts up to 1000 characters and rejects more", () => {
    expect(requestNoteError("")).toBeNull();
    expect(requestNoteError("a".repeat(MODULE_REQUEST_NOTE_MAX))).toBeNull();
    expect(requestNoteError("a".repeat(MODULE_REQUEST_NOTE_MAX + 1))).toBe(
      "modules.requests.note_too_long",
    );
  });
});
