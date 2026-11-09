import { describe, expect, it } from "vitest";

import { recipientPhoneE164, recipientSchema } from "./new-recipient-dialog";

const t = (key: string) => key;
const base = {
  name: "Perakende",
  phone: "0555 111 22 33",
  kind: "retail_point" as const,
  country_id: "1",
  province_id: "",
  district_id: "",
  address: "",
  tax_no: "",
  tax_office: "",
};

describe("new recipient (TEC-1062)", () => {
  it("normalizes the phone to E.164 in the picked country", () => {
    expect(recipientPhoneE164("0555 111 22 33", "TR")).toBe("+905551112233");
    expect(recipientPhoneE164("+49 30 901820", null)).toBe("+4930901820");
    expect(recipientPhoneE164("123", "TR")).toBeNull();
    expect(recipientPhoneE164("", "TR")).toBeNull();
  });

  it("validates the phone and the tax number", () => {
    const schema = recipientSchema(t, () => "TR");
    expect(schema.safeParse(base).success).toBe(true);
    const badPhone = schema.safeParse({ ...base, phone: "12" });
    expect(badPhone.success).toBe(false);
    expect(badPhone.error?.issues[0]?.path).toEqual(["phone"]);
    expect(schema.safeParse({ ...base, tax_no: "123" }).success).toBe(false);
    expect(schema.safeParse({ ...base, tax_no: "12345678901" }).success).toBe(
      true,
    );
    expect(schema.safeParse({ ...base, name: " " }).success).toBe(false);
  });
});
