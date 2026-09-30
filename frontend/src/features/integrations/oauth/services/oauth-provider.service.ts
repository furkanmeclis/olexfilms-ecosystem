import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type OAuthProviderSlug = "google" | "facebook" | "apple";

export type OAuthProviderSettings =
  components["schemas"]["OAuthProviderSettings"];
export type OAuthProviderSettingsPatch =
  components["schemas"]["PatchOAuthProviderSettingsRequest"];

export const oauthProviderService = {
  async getSettings(provider: OAuthProviderSlug) {
    if (provider === "google") {
      return unwrap<OAuthProviderSettings>(
        await apiClient.GET("/v1/platform/integrations/google"),
      );
    }
    if (provider === "facebook") {
      return unwrap<OAuthProviderSettings>(
        await apiClient.GET("/v1/platform/integrations/facebook"),
      );
    }
    return unwrap<OAuthProviderSettings>(
      await apiClient.GET("/v1/platform/integrations/apple"),
    );
  },

  async patchSettings(
    provider: OAuthProviderSlug,
    body: OAuthProviderSettingsPatch,
  ) {
    if (provider === "google") {
      return unwrap<OAuthProviderSettings>(
        await apiClient.PATCH("/v1/platform/integrations/google", { body }),
      );
    }
    if (provider === "facebook") {
      return unwrap<OAuthProviderSettings>(
        await apiClient.PATCH("/v1/platform/integrations/facebook", { body }),
      );
    }
    return unwrap<OAuthProviderSettings>(
      await apiClient.PATCH("/v1/platform/integrations/apple", { body }),
    );
  },
};
