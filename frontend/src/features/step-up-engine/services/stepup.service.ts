import {
  normalizePasskeyRequestOptions,
  type PasskeyOptionsPayload,
} from "@/features/step-up-engine/lib/passkey-options";
import type {
  StepUpGrant,
  StepUpPolicy,
  StepUpStatus,
} from "@/features/step-up-engine/types";
import { platformRequest } from "@/lib/api/platform-request";

export const stepUpService = {
  getStatus() {
    return platformRequest<StepUpStatus>("GET", "/v1/auth/step-up");
  },

  verifyPassword(password: string) {
    return platformRequest<StepUpGrant>("POST", "/v1/auth/step-up/password", {
      body: { password },
    });
  },

  async passkeyOptions() {
    const raw = await platformRequest<PasskeyOptionsPayload>(
      "POST",
      "/v1/auth/step-up/passkey/options",
    );
    return normalizePasskeyRequestOptions(raw);
  },

  passkeyVerify(credential: unknown) {
    return platformRequest<StepUpGrant>(
      "POST",
      "/v1/auth/step-up/passkey/verify",
      { body: credential },
    );
  },

  verifyTotp(code: string) {
    return platformRequest<StepUpGrant>("POST", "/v1/auth/step-up/totp", {
      body: { code },
    });
  },

  getPolicy() {
    return platformRequest<StepUpPolicy>("GET", "/v1/platform/access/settings");
  },

  patchPolicy(body: Partial<StepUpPolicy>) {
    return platformRequest<StepUpPolicy>(
      "PATCH",
      "/v1/platform/access/settings",
      { body },
    );
  },
};
