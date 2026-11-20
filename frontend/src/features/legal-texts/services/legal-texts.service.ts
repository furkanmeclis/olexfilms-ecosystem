import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type LegalText = components["schemas"]["LegalText"];
export type LegalTextAdmin = components["schemas"]["LegalTextAdmin"];
export type LegalTextPublish = components["schemas"]["LegalTextPublish"];
/**
 * Editable kinds: the AI guidelines (asked after sign-in, K22) and the
 * public texts of TEC-1257 (privacy policy, KVKK notice, terms of use; read
 * without a session from the sign-in screen and the profile).
 */
export const LEGAL_TEXT_KINDS = [
  "ai_guidelines",
  "privacy_policy",
  "kvkk_notice",
  "terms_of_use",
] as const;
export type LegalTextKind = (typeof LEGAL_TEXT_KINDS)[number];

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
