import { describe, expect, it } from "vitest";

import { normalizeVin, validateVin, vinErrorKey } from "./vin";

describe("normalizeVin", () => {
  it("upper-cases and drops spaces and dashes like the backend", () => {
    expect(normalizeVin(" wvw-zzz 1jz3w386752 ")).toBe("WVWZZZ1JZ3W386752");
    expect(normalizeVin("wvw\tzzz1jz3w386752")).toBe("WVWZZZ1JZ3W386752");
  });
});

describe("validateVin", () => {
  it("accepts a 17 character VIN without I, O, Q", () => {
    expect(validateVin("WVWZZZ1JZ3W386752")).toBeNull();
    expect(validateVin("wvwzzz1jz3w386752")).toBeNull();
    expect(validateVin("WVW-ZZZ 1JZ3W386752")).toBeNull();
  });

  it("treats empty as valid unless required", () => {
    expect(validateVin("")).toBeNull();
    expect(validateVin("  ")).toBeNull();
    expect(validateVin("", { required: true })).toBe("required");
  });

  it("rejects a wrong length", () => {
    expect(validateVin("WVWZZZ1JZ3W38675")).toBe("length");
    expect(validateVin("WVWZZZ1JZ3W3867521")).toBe("length");
  });

  it("rejects I, O and Q", () => {
    expect(validateVin("WVWZZZ1JZ3W38675I")).toBe("forbidden_letters");
    expect(validateVin("WVWZZZ1JZ3W38675o")).toBe("forbidden_letters");
    expect(validateVin("QVWZZZ1JZ3W386752")).toBe("forbidden_letters");
  });

  it("rejects other characters", () => {
    expect(validateVin("WVWZZZ1JZ3W38675*")).toBe("chars");
    expect(validateVin("WVWZZZ1JZ3W38675Ş")).toBe("chars");
  });

  it("maps errors to i18n keys", () => {
    expect(vinErrorKey("length")).toBe("services.vin.errors.length");
  });
});
