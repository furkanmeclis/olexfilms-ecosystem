import { describe, expect, it } from "vitest";

import {
  ownerFromServices,
  personName,
  summarizeVehicle360,
} from "./vehicle-360";

const customer = (uuid: string, name: string) => ({
  uuid,
  name,
  surname: "Yılmaz",
  phone: null,
  anonymized: false,
});

describe("summarizeVehicle360", () => {
  it("is empty without records", () => {
    expect(
      summarizeVehicle360({ services: [], warranties: [], claims: [] }),
    ).toEqual({
      serviceCount: 0,
      lastServiceAt: null,
      activeWarranties: 0,
      nextWarrantyEnd: null,
      openClaims: 0,
    });
  });

  it("counts live services and picks the latest date", () => {
    const s = summarizeVehicle360({
      services: [
        {
          status: "completed",
          created_at: "2026-01-01T00:00:00Z",
          completed_at: "2026-01-05T00:00:00Z",
        },
        { status: "processing", created_at: "2026-03-01T00:00:00Z" },
        { status: "cancelled", created_at: "2026-09-01T00:00:00Z" },
      ],
      warranties: [],
      claims: [],
    });
    expect(s.serviceCount).toBe(2);
    expect(s.lastServiceAt).toBe("2026-03-01T00:00:00Z");
  });

  it("prefers completed_at over created_at", () => {
    const s = summarizeVehicle360({
      services: [
        {
          status: "completed",
          created_at: "2026-01-01T00:00:00Z",
          completed_at: "2026-06-01T00:00:00Z",
        },
        { status: "draft", created_at: "2026-04-01T00:00:00Z" },
      ],
      warranties: [],
      claims: [],
    });
    expect(s.lastServiceAt).toBe("2026-06-01T00:00:00Z");
  });

  it("counts active warranties with the soonest end and open claims", () => {
    const s = summarizeVehicle360({
      services: [],
      warranties: [
        { status: "active", end_at: "2030-01-01T00:00:00Z" },
        { status: "active", end_at: "2028-01-01T00:00:00Z" },
        { status: "expired", end_at: "2020-01-01T00:00:00Z" },
        { status: "void", end_at: "2027-01-01T00:00:00Z" },
      ],
      claims: [
        { status: "submitted" },
        { status: "center_review" },
        { status: "rejected" },
        { status: "closed" },
      ],
    });
    expect(s.activeWarranties).toBe(2);
    expect(s.nextWarrantyEnd).toBe("2028-01-01T00:00:00Z");
    expect(s.openClaims).toBe(2);
  });
});

describe("ownerFromServices", () => {
  it("takes the newest service of the current customer", () => {
    const owner = ownerFromServices(
      [
        {
          status: "completed",
          created_at: "2026-01-01T00:00:00Z",
          customer: customer("c1", "Ali"),
        },
        {
          status: "completed",
          created_at: "2026-05-01T00:00:00Z",
          customer: customer("c2", "Eski"),
        },
        {
          status: "draft",
          created_at: "2026-03-01T00:00:00Z",
          customer: customer("c1", "Ali Can"),
        },
      ],
      "c1",
    );
    expect(owner?.name).toBe("Ali Can");
  });

  it("is null when no service belongs to the customer", () => {
    expect(
      ownerFromServices(
        [
          {
            status: "completed",
            created_at: "2026-01-01T00:00:00Z",
            customer: customer("c2", "X"),
          },
        ],
        "c1",
      ),
    ).toBeNull();
  });
});

describe("personName", () => {
  it("joins name and surname", () => {
    expect(personName({ name: "Ali", surname: "" })).toBe("Ali");
    expect(personName({ name: "Ali", surname: "Can" })).toBe("Ali Can");
    expect(personName(null)).toBe("");
  });
});
