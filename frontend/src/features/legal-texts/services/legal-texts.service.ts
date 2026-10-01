import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type LegalText = components["schemas"]["LegalText"];
export type LegalTextAdmin = components["schemas"]["LegalTextAdmin"];
export type LegalTextPublish = components["schemas"]["LegalTextPublish"];
export type LegalTextKind = "ai_guidelines";

/** Admin editor of the portal legal texts (`platform.legal_texts.write`). */
export const legalTextsService = {
  async get(kind: LegalTextKind) {
    return unwrap<LegalTextAdmin>(
      await apiClient.GET("/v1/platform/legal-texts/{kind}", {
        params: { path: { kind } },
      }),
    );
  },

  async publish(kind: LegalTextKind, locale: string, body: string) {
    return unwrap<LegalTextPublish>(
      await apiClient.PUT("/v1/platform/legal-texts/{kind}", {
        params: { path: { kind } },
        body: { locale, body },
      }),
    );
  },
};
