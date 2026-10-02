import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import {
  resolveCustomerDetailAccess,
  resolveCustomerListAccess,
} from "@/features/customers/lib/access";
import {
  EMPTY_CUSTOMER_FORM,
  EMPTY_VEHICLE_FORM,
  buildCustomerCreate,
  buildCustomerUpdate,
  buildVehicleCreate,
  buildVehicleUpdate,
  plateError,
  validateCustomerForm,
  validateVehicleForm,
} from "@/features/customers/lib/form";

const can = (granted: string[]) => (p: string) => granted.includes(p);

// Seeded formats (backend migrations): TR and DE.
const FORMATS = [
  {
    country_iso2: "TR",
    regex: "^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$",
    example: "34 ABC 123",
  },
  {
    country_iso2: "DE",
    regex: "^[A-ZÄÖÜ]{1,3}[A-Z]{1,2}[1-9][0-9]{0,3}[EH]?$",
    example: "B AB 1234",
  },
];

describe("customer access (TEC-163)", () => {
  const live = { editable: true, anonymized: false, status: "active" as const };

  it("list needs customers.read, create customers.write", () => {
    expect(resolveCustomerListAccess(can([]))).toEqual({
      canRead: false,
      canCreate: false,
    });
    expect(
      resolveCustomerListAccess(
        can([Permission.CustomersRead, Permission.CustomersWrite]),
      ),
    ).toEqual({ canRead: true, canCreate: true });
  });

  it("privacy actions need customers.anonymize in a center", () => {
    const g = can([Permission.CustomersRead, Permission.CustomersAnonymize]);
    expect(resolveCustomerDetailAccess(g, "center", live)).toMatchObject({
      canAnonymize: true,
      canExport: true,
    });
    expect(resolveCustomerDetailAccess(g, "distributor", live)).toMatchObject({
      canAnonymize: false,
      canExport: false,
    });
    expect(resolveCustomerDetailAccess(can([]), "center", live)).toMatchObject({
      canAnonymize: false,
      canExport: false,
    });
    expect(
      resolveCustomerDetailAccess(g, "center", {
        editable: false,
        anonymized: true,
        status: "anonymized",
      }),
    ).toMatchObject({ canAnonymize: false, canExport: false });
  });

  it("upgrade: customers.write + organizations.read, not for dealers", () => {
    const g = can([Permission.CustomersWrite, Permission.OrganizationsRead]);
    expect(resolveCustomerDetailAccess(g, "center", live).canUpgrade).toBe(
      true,
    );
    expect(resolveCustomerDetailAccess(g, "distributor", live).canUpgrade).toBe(
      true,
    );
    expect(resolveCustomerDetailAccess(g, "dealer", live).canUpgrade).toBe(
      false,
    );
    expect(
      resolveCustomerDetailAccess(g, "center", {
        ...live,
        status: "disabled",
      }).canUpgrade,
    ).toBe(false);
  });

  it("edit and vehicle writes follow `editable`", () => {
    const g = can([
      Permission.CustomersWrite,
      Permission.VehiclesRead,
      Permission.VehiclesWrite,
    ]);
    expect(resolveCustomerDetailAccess(g, "dealer", live)).toMatchObject({
      canEdit: true,
      canWriteVehicles: true,
    });
    expect(
      resolveCustomerDetailAccess(g, "dealer", { ...live, editable: false }),
    ).toMatchObject({ canEdit: false, canWriteVehicles: false });
  });
});

describe("customer form validation", () => {
  it("requires phone (create) and name", () => {
    expect(validateCustomerForm(EMPTY_CUSTOMER_FORM, "create")).toEqual({
      phone: "phone_required",
      name: "name_required",
    });
    expect(validateCustomerForm(EMPTY_CUSTOMER_FORM, "edit")).toEqual({
      name: "name_required",
    });
  });

  it("checks phone digits, e-mail, identity numbers and company", () => {
    const e = validateCustomerForm(
      {
        ...EMPTY_CUSTOMER_FORM,
        phone: "12",
        name: "Ayşe",
        email: "nope",
        national_id: "12",
        type: "corporate",
      },
      "create",
    );
    expect(e).toEqual({
      phone: "phone_invalid",
      email: "email_invalid",
      national_id: "identity_invalid",
      company_name: "company_required",
    });
    expect(
      validateCustomerForm(
        {
          ...EMPTY_CUSTOMER_FORM,
          phone: "+90 555 123 45 67",
          name: "Ayşe",
          national_id: "123 456 789 01",
        },
        "create",
      ),
    ).toEqual({});
  });

  it("rejects names over 100 characters", () => {
    expect(
      validateCustomerForm(
        { ...EMPTY_CUSTOMER_FORM, phone: "5551234567", name: "a".repeat(101) },
        "create",
      ).name,
    ).toBe("too_long");
  });

  it("create body drops empty fields and normalizes identities", () => {
    expect(
      buildCustomerCreate({
        ...EMPTY_CUSTOMER_FORM,
        phone: " 0555 123 45 67 ",
        name: "  Ayşe   Yılmaz ",
        email: "A@B.COM",
        national_id: "123-456-789.01",
      }),
    ).toEqual({
      phone: "0555 123 45 67",
      name: "Ayşe Yılmaz",
      type: "individual",
      email: "a@b.com",
      national_id: "12345678901",
    });
  });

  it("update body has only the changed fields; cleared ones are null", () => {
    const original = {
      ...EMPTY_CUSTOMER_FORM,
      name: "Ayşe",
      surname: "Yılmaz",
      email: "a@b.com",
    };
    expect(
      buildCustomerUpdate({ ...original, surname: "", name: "Ayşe" }, original),
    ).toEqual({ surname: null });
    expect(buildCustomerUpdate(original, original)).toEqual({});
  });
});

describe("plate format (geo.ValidatePlate rules)", () => {
  it("matches the compact plate against the country regex", () => {
    expect(plateError("34 abc 123", "TR", FORMATS)).toBeNull();
    expect(plateError("34-ABC-123", "tr", FORMATS)).toBeNull();
    expect(plateError("99 ABC 123", "TR", FORMATS)).toBe("plate_invalid");
    expect(plateError("B AB 1234", "DE", FORMATS)).toBeNull();
    expect(plateError("B AB 0123", "DE", FORMATS)).toBe("plate_invalid");
  });

  it("requires a plate, limits length, leaves unknown countries to the server", () => {
    expect(plateError("  ", "TR", FORMATS)).toBe("plate_required");
    expect(plateError("A".repeat(21), "XX", FORMATS)).toBe("plate_too_long");
    expect(plateError("ANYTHING1", "XX", FORMATS)).toBeNull();
  });

  it("validates the whole vehicle form", () => {
    expect(
      validateVehicleForm(
        {
          ...EMPTY_VEHICLE_FORM,
          plate: "99 ABC 123",
          plate_country: "TR",
          model_year: "1800",
          vin: "ABC",
        },
        FORMATS,
        2026,
      ),
    ).toEqual({
      plate: "plate_invalid",
      model_year: "year_invalid",
      vin: "vin_invalid",
    });
    expect(
      validateVehicleForm(
        { ...EMPTY_VEHICLE_FORM, plate: "34 ABC 123", plate_country: "" },
        FORMATS,
      ),
    ).toEqual({ plate_country: "country_required" });
  });

  it("builds create and update bodies", () => {
    const v = {
      ...EMPTY_VEHICLE_FORM,
      plate: " 34 ABC 123 ",
      plate_country: "tr",
      brand_uuid: "b1",
      model_uuid: "m1",
      model_year: "2022",
      vin: "wvw-zzz1kz-bw000001",
    };
    expect(buildVehicleCreate("c1", v)).toEqual({
      customer_uuid: "c1",
      plate: "34 ABC 123",
      plate_country: "TR",
      car_brand_uuid: "b1",
      car_model_uuid: "m1",
      model_year: 2022,
      vin: "WVWZZZ1KZBW000001",
    });
    expect(
      buildVehicleUpdate({ ...v, model_uuid: "", model_year: "" }, v),
    ).toEqual({ car_model_uuid: null, model_year: null });
  });
});
