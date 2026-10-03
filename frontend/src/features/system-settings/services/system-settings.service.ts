import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type SystemSetting = Schemas["SystemSetting"];
export type SystemSettingValue = Schemas["SystemSettingValue"];
export type SystemSettingGroup = SystemSetting["group"];

const enc = encodeURIComponent;

/** TEC-215 global system settings store (platform.settings.read/write). */
export const systemSettingsService = {
  list() {
    return platformRequest<{ items: SystemSetting[] }>(
      "GET",
      "/v1/platform/system-settings",
    );
  },

  get(key: string) {
    return platformRequest<SystemSetting>(
      "GET",
      `/v1/platform/system-settings/${enc(key)}`,
    );
  },

  /** 400 VALIDATION_ERROR (details[].field = "value") on schema mismatch. */
  put(key: string, value: SystemSettingValue) {
    return platformRequest<SystemSetting>(
      "PUT",
      `/v1/platform/system-settings/${enc(key)}`,
      { body: { value } },
    );
  },

  /** Drops the override; the key returns to its catalog default. */
  reset(key: string) {
    return platformRequest<SystemSetting>(
      "DELETE",
      `/v1/platform/system-settings/${enc(key)}`,
    );
  },
};
