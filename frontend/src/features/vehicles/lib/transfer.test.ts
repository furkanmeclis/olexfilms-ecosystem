import { describe, expect, it } from "vitest";

import {
  isTransferCode,
  pendingTransfer,
  transferStatusTone,
  verifyBody,
} from "./transfer";

const base = {
  status: "pending",
  from_verified: false,
  to_verified: false,
  new_owner_known: true,
};
const empty = { fromCode: "", toCode: "", name: "", surname: "" };

describe("vehicle transfer helpers", () => {
  it("finds the pending transfer", () => {
    expect(pendingTransfer(undefined)).toBeUndefined();
    expect(
      pendingTransfer([
        { uuid: "a", status: "completed" },
        { uuid: "b", status: "pending" },
      ])?.uuid,
    ).toBe("b");
  });

  it("accepts only 6 digit codes", () => {
    expect(isTransferCode("123456")).toBe(true);
    expect(isTransferCode(" 123456 ")).toBe(true);
    expect(isTransferCode("12345")).toBe(false);
    expect(isTransferCode("12345a")).toBe(false);
  });

  it("sends only open codes and the name of a new owner", () => {
    expect(
      verifyBody(base, { ...empty, fromCode: "111111", toCode: "222222" }),
    ).toEqual({ from_code: "111111", to_code: "222222" });
    expect(
      verifyBody(
        { ...base, from_verified: true },
        { ...empty, fromCode: "111111", toCode: "222222", name: "Ali" },
      ),
    ).toEqual({ to_code: "222222" });
    expect(
      verifyBody(
        { ...base, new_owner_known: false },
        { fromCode: "", toCode: "222222", name: " Ali ", surname: "" },
      ),
    ).toEqual({ to_code: "222222", new_owner_name: "Ali" });
  });

  it("maps statuses to tones", () => {
    expect(transferStatusTone("completed")).toBe("success");
    expect(transferStatusTone("pending")).toBe("warning");
    expect(transferStatusTone("expired")).toBe("default");
  });
});
