import type { AppLocale } from "@/config/i18n";
import { apiConfig } from "@/config/api";

export type AuthMethodFlags = {
  login: boolean;
  register?: boolean;
};

export type AppPublicConfig = {
  mode: string;
  name: string;
  vapid_configured: boolean;
  github_auth_enabled: boolean;
  registration_enabled: boolean;
  auth_methods: {
    password: AuthMethodFlags;
    passkey: { login: boolean };
    github: AuthMethodFlags;
    google: AuthMethodFlags;
    facebook: AuthMethodFlags;
    apple: AuthMethodFlags;
  };
};

const defaultMethods = (): AppPublicConfig["auth_methods"] => ({
  password: { login: true, register: false },
  passkey: { login: true },
  github: { login: false, register: false },
  google: { login: false, register: false },
  facebook: { login: false, register: false },
  apple: { login: false, register: false },
});

export async function fetchAppPublicConfig(
  locale: AppLocale = "en",
): Promise<AppPublicConfig> {
  const response = await fetch(`${apiConfig.baseUrl}/v1/app/config`, {
    headers: {
      Accept: "application/json",
      "Accept-Language": locale,
    },
    next: { revalidate: 300 }, // cache for 5 min; auth providers don't change often
  });

  if (!response.ok) {
    throw new Error("Failed to load app config");
  }

  const json = (await response.json()) as {
    success?: boolean;
    data?: Partial<AppPublicConfig>;
  };

  if (!json.data) {
    throw new Error("Invalid app config response");
  }

  const methods = {
    ...defaultMethods(),
    ...(json.data.auth_methods ?? {}),
  };

  return {
    mode: json.data.mode ?? "platform",
    name: json.data.name ?? "",
    vapid_configured: Boolean(json.data.vapid_configured),
    github_auth_enabled:
      methods.github.login || Boolean(json.data.github_auth_enabled),
    registration_enabled: Boolean(json.data.registration_enabled),
    auth_methods: methods,
  };
}
