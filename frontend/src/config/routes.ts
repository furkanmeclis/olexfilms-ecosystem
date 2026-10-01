export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  tenant: {
    home: (slug: string) => `/t/${slug}`,
    login: (slug: string) => `/t/${slug}/login`,
    profile: {
      root: (slug: string) => `/t/${slug}/profile`,
      password: (slug: string) => `/t/${slug}/profile/password`,
      preferences: (slug: string) => `/t/${slug}/profile/preferences`,
    },
    exports: {
      root: (slug: string) => `/t/${slug}/exports`,
      detail: (slug: string, uuid: string) => `/t/${slug}/exports/${uuid}`,
    },
    imports: {
      root: (slug: string) => `/t/${slug}/imports`,
      detail: (slug: string, uuid: string) => `/t/${slug}/imports/${uuid}`,
    },
    settings: {
      root: (slug: string) => `/t/${slug}/settings`,
    },
    features: {
      root: (slug: string) => `/t/${slug}/features`,
    },
  },
  guest: {
    login: "/platform/login",
    register: "/platform/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  cms: {
    root: "/",
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    roles: {
      root: "/platform/roles",
      create: "/platform/roles/create",
      detail: (uuid: string) => `/platform/roles/${uuid}`,
      edit: (uuid: string) => `/platform/roles/${uuid}/edit`,
    },
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
      center: "/platform/notification-center",
    },
    imports: {
      root: "/platform/imports",
      detail: (uuid: string) => `/platform/imports/${uuid}`,
    },
    exports: {
      root: "/platform/exports",
      detail: (uuid: string) => `/platform/exports/${uuid}`,
    },
    settings: {
      root: "/platform/settings",
    },
    access: {
      root: "/platform/access",
    },
    authSettings: {
      root: "/platform/auth/settings",
    },
    integrations: {
      github: "/platform/integrations/github",
      google: "/platform/integrations/google",
      facebook: "/platform/integrations/facebook",
      apple: "/platform/integrations/apple",
      whatsapp: "/platform/integrations/whatsapp",
    },
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
      edit: (uuid: string) => `/platform/organizations/${uuid}/edit`,
    },
    documentTemplates: {
      root: "/platform/document-templates",
      edit: (uuid: string) => `/platform/document-templates/${uuid}`,
    },
    activity: {
      root: "/platform/activity",
    },
    logs: {
      root: "/platform/logs",
    },
    storage: {
      root: "/platform/storage",
    },
    modules: {
      root: "/platform/modules",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;
