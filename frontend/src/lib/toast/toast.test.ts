import { beforeEach, describe, expect, it, vi } from "vitest";

const sonner = vi.hoisted(() => ({
  success: vi.fn(() => "s"),
  error: vi.fn(() => "e"),
  warning: vi.fn(() => "w"),
  info: vi.fn(() => "i"),
  message: vi.fn(() => "m"),
  loading: vi.fn(() => "l"),
  dismiss: vi.fn(),
  promise: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: sonner }));

import { ApiError } from "@/lib/api";

import { apiErrorToast } from "./api-error";
import { TOAST_DURATIONS, toast } from "./index";

const dict: Record<string, string> = {
  "common.error_generic": "Bir hata oluştu",
  "common.error_server": "Sunucu hatası",
  "common.error_reference": "Destek için başvuru numarası: {{id}}",
  "common.error_forbidden": "Yetkiniz yok",
  "common.error_validation": "Formu kontrol edin",
  "errors.feature_disabled": "Modül kapalı",
  "errors.codes.CUSTOMER_MERGE_SELF": "Kendisiyle birleştirilemez.",
  "auth.session_expired": "Oturum doldu",
};
const t = (key: string, params?: Record<string, string | number>) => {
  let v = dict[key] ?? key;
  for (const [k, p] of Object.entries(params ?? {}))
    v = v.replace(`{{${k}}}`, String(p));
  return v;
};

const apiErr = (
  status: number,
  code: string,
  message = "",
  requestId?: string,
) => new ApiError({ status, code, message, requestId });

describe("toast", () => {
  beforeEach(() => vi.clearAllMocks());

  it("uses kind durations and a dedupe id for plain text", () => {
    toast.success("Kaydedildi");
    expect(sonner.success).toHaveBeenCalledWith("Kaydedildi", {
      duration: TOAST_DURATIONS.success,
      id: "success:Kaydedildi",
    });
    toast.error("Olmadı");
    expect(sonner.error).toHaveBeenCalledWith("Olmadı", {
      duration: 6000,
      id: "error:Olmadı",
    });
  });

  it("keeps caller id, duration and action; dedupe can be turned off", () => {
    const action = { label: "Geri al", onClick: vi.fn() };
    toast.success("Silindi", { id: 7, duration: 10000, action });
    expect(sonner.success).toHaveBeenCalledWith("Silindi", {
      duration: 10000,
      id: 7,
      action,
    });
    toast.info("Kuyrukta", { dedupe: false });
    expect(sonner.info).toHaveBeenCalledWith("Kuyrukta", { duration: 4000 });
  });

  it("loading returns the id for a later result", () => {
    const id = toast.loading("Yükleniyor");
    expect(id).toBe("l");
    toast.success("Bitti", { id });
    expect(sonner.success).toHaveBeenLastCalledWith("Bitti", {
      duration: 4000,
      id: "l",
    });
  });

  it("apiError maps code to user text", () => {
    toast.apiError(apiErr(409, "CUSTOMER_MERGE_SELF", "merge self"), t);
    expect(sonner.error).toHaveBeenCalledWith("Kendisiyle birleştirilemez.", {
      duration: 6000,
      id: "error:Kendisiyle birleştirilemez.",
    });
  });
});

describe("apiErrorToast", () => {
  it("silences limit codes and dedupes feature-disabled", () => {
    expect(apiErrorToast(apiErr(402, "LIMIT_REACHED"), t)).toBeNull();
    expect(apiErrorToast(apiErr(403, "FEATURE_DISABLED"), t)).toEqual({
      message: "Modül kapalı",
      id: "feature-disabled",
    });
  });

  it("puts the request id into the description, not the title", () => {
    expect(
      apiErrorToast(apiErr(500, "INTERNAL_ERROR", "boom", "req-1"), t),
    ).toEqual({
      message: "Sunucu hatası",
      description: "Destek için başvuru numarası: req-1",
    });
  });

  it("falls back to status texts and server validation text", () => {
    expect(apiErrorToast(apiErr(403, "FORBIDDEN"), t)?.message).toBe(
      "Yetkiniz yok",
    );
    expect(
      apiErrorToast(apiErr(422, "VALIDATION_ERROR", "Ad zorunlu"), t)?.message,
    ).toBe("Ad zorunlu");
    expect(apiErrorToast(apiErr(400, "UNKNOWN_CODE"), t)?.message).toBe(
      "Formu kontrol edin",
    );
    expect(apiErrorToast(new Error("x"), t)?.message).toBe("Bir hata oluştu");
  });
});
