import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type GitHubIntegrationSettings =
  components["schemas"]["GitHubIntegrationSettings"];
export type GitHubSettingsPatch =
  components["schemas"]["PatchGitHubIntegrationSettingsRequest"];

export const githubIntegrationService = {
  async getSettings() {
    return unwrap<GitHubIntegrationSettings>(
      await apiClient.GET("/v1/platform/integrations/github"),
    );
  },

  async patchSettings(body: GitHubSettingsPatch) {
    return unwrap<GitHubIntegrationSettings>(
      await apiClient.PATCH("/v1/platform/integrations/github", { body }),
    );
  },
};
