import type { AppSettings, ExportJobScope } from "@/features/io/types";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type PatchSettingsRequest = Partial<Omit<AppSettings, "logo_url">>;

function settingsBase(scope: ExportJobScope = "platform") {
  return scope === "tenant" ? "/v1/tenant/settings" : "/v1/platform/settings";
}

export const settingsService = {
  async get(scope: ExportJobScope = "platform") {
    return platformRequest<AppSettings>("GET", settingsBase(scope));
  },

  async patch(body: PatchSettingsRequest, scope: ExportJobScope = "platform") {
    return platformRequest<AppSettings>("PATCH", settingsBase(scope), {
      body,
    });
  },

  async uploadLogo(file: File, scope: ExportJobScope = "platform") {
    const form = new FormData();
    form.append("logo", file);
    return platformFormRequest<AppSettings>(
      "PUT",
      `${settingsBase(scope)}/logo`,
      form,
    );
  },

  async deleteLogo(scope: ExportJobScope = "platform") {
    return platformRequest<AppSettings>(
      "DELETE",
      `${settingsBase(scope)}/logo`,
    );
  },
};
