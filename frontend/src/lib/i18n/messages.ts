import type { AppLocale } from "@/config/i18n";

import enAccess from "@/locales/en/access.json";
import enModules from "@/locales/en/modules.json";
import enActivity from "@/locales/en/activity.json";
import enAuth from "@/locales/en/auth.json";
import enBranding from "@/locales/en/branding.json";
import enBulk from "@/locales/en/bulk.json";
import enChart from "@/locales/en/chart.json";
import enCms from "@/locales/en/cms.json";
import enCommon from "@/locales/en/common.json";
import enDashboard from "@/locales/en/dashboard.json";
import enEditor from "@/locales/en/editor.json";
import enEntity from "@/locales/en/entity.json";
import enDocuments from "@/locales/en/documents.json";
import enExports from "@/locales/en/exports.json";
import enForm from "@/locales/en/form.json";
import enImports from "@/locales/en/imports.json";
import enIntegrations from "@/locales/en/integrations.json";
import enLogs from "@/locales/en/logs.json";
import enLayout from "@/locales/en/layout.json";
import enNotifications from "@/locales/en/notifications.json";
import enOrganizations from "@/locales/en/organizations.json";
import enPermissions from "@/locales/en/permissions.json";
import enRoles from "@/locales/en/roles.json";
import enSearch from "@/locales/en/search.json";
import enErrors from "@/locales/en/errors.json";
import enRealtime from "@/locales/en/realtime.json";
import enSettings from "@/locales/en/settings.json";
import enStepup from "@/locales/en/stepup.json";
import enStorage from "@/locales/en/storage.json";
import enTable from "@/locales/en/table.json";
import enUsers from "@/locales/en/users.json";
import enRegister from "@/locales/en/register.json";
import trAccess from "@/locales/tr/access.json";
import trModules from "@/locales/tr/modules.json";
import trActivity from "@/locales/tr/activity.json";
import trAuth from "@/locales/tr/auth.json";
import trBranding from "@/locales/tr/branding.json";
import trBulk from "@/locales/tr/bulk.json";
import trChart from "@/locales/tr/chart.json";
import trCms from "@/locales/tr/cms.json";
import trCommon from "@/locales/tr/common.json";
import trDashboard from "@/locales/tr/dashboard.json";
import trEditor from "@/locales/tr/editor.json";
import trEntity from "@/locales/tr/entity.json";
import trDocuments from "@/locales/tr/documents.json";
import trExports from "@/locales/tr/exports.json";
import trForm from "@/locales/tr/form.json";
import trImports from "@/locales/tr/imports.json";
import trIntegrations from "@/locales/tr/integrations.json";
import trLogs from "@/locales/tr/logs.json";
import trLayout from "@/locales/tr/layout.json";
import trNotifications from "@/locales/tr/notifications.json";
import trOrganizations from "@/locales/tr/organizations.json";
import trPermissions from "@/locales/tr/permissions.json";
import trRoles from "@/locales/tr/roles.json";
import trSearch from "@/locales/tr/search.json";
import trErrors from "@/locales/tr/errors.json";
import trRealtime from "@/locales/tr/realtime.json";
import trSettings from "@/locales/tr/settings.json";
import trStepup from "@/locales/tr/stepup.json";
import trStorage from "@/locales/tr/storage.json";
import trTable from "@/locales/tr/table.json";
import trUsers from "@/locales/tr/users.json";
import trRegister from "@/locales/tr/register.json";

export type MessageDictionary = Record<string, string>;

const catalogs: Record<AppLocale, Record<string, MessageDictionary>> = {
  tr: {
    common: trCommon,
    layout: trLayout,
    editor: trEditor,
    table: trTable,
    form: trForm,
    chart: trChart,
    auth: trAuth,
    branding: trBranding,
    bulk: trBulk,
    entity: trEntity,
    permissions: trPermissions,
    realtime: trRealtime,
    cms: trCms,
    roles: trRoles,
    search: trSearch,
    errors: trErrors,
    users: trUsers,
    notifications: trNotifications,
    dashboard: trDashboard,
    activity: trActivity,
    documents: trDocuments,
    exports: trExports,
    imports: trImports,
    logs: trLogs,
    settings: trSettings,
    storage: trStorage,
    access: trAccess,
    modules: trModules,
    integrations: trIntegrations,
    organizations: trOrganizations,
    register: trRegister,
    stepup: trStepup,
  },
  en: {
    common: enCommon,
    layout: enLayout,
    editor: enEditor,
    table: enTable,
    form: enForm,
    chart: enChart,
    auth: enAuth,
    branding: enBranding,
    bulk: enBulk,
    entity: enEntity,
    permissions: enPermissions,
    realtime: enRealtime,
    cms: enCms,
    roles: enRoles,
    search: enSearch,
    errors: enErrors,
    users: enUsers,
    notifications: enNotifications,
    dashboard: enDashboard,
    activity: enActivity,
    documents: enDocuments,
    exports: enExports,
    imports: enImports,
    logs: enLogs,
    settings: enSettings,
    storage: enStorage,
    access: enAccess,
    modules: enModules,
    integrations: enIntegrations,
    organizations: enOrganizations,
    register: enRegister,
    stepup: enStepup,
  },
};
export function loadMessages(locale: AppLocale) {
  return catalogs[locale];
}

export function translate(
  locale: AppLocale,
  key: string,
  params?: Record<string, string | number>,
  fallbackLocale: AppLocale = "en",
): string {
  const [ns, ...rest] = key.split(".");
  const path = rest.join(".");
  const primary = catalogs[locale]?.[ns]?.[path];
  const fallback = catalogs[fallbackLocale]?.[ns]?.[path];
  if (primary === undefined)
    recordMissingKey(locale, key, fallback !== undefined);
  let text = primary ?? fallback ?? key;

  if (params) {
    for (const [k, v] of Object.entries(params)) {
      text = text.replace(new RegExp(`{{\\s*${k}\\s*}}`, "g"), String(v));
    }
  }

  return text;
}

/** Simple plural helper: key_one / key_other */
export function translatePlural(
  locale: AppLocale,
  key: string,
  count: number,
  params?: Record<string, string | number>,
) {
  const suffix = count === 1 ? "one" : "other";
  return translate(locale, `${key}_${suffix}`, { count, ...params });
}

/**
 * Development aid: remembers keys the active locale could not resolve so a
 * crawler (or `window.__i18nMissing` in devtools) can list them. No-op in
 * production builds.
 */
function recordMissingKey(
  locale: AppLocale,
  key: string,
  hasFallback: boolean,
) {
  if (process.env.NODE_ENV === "production" || typeof window === "undefined")
    return;
  const w = window as unknown as { __i18nMissing?: Record<string, string> };
  w.__i18nMissing ??= {};
  w.__i18nMissing[`${locale}:${key}`] = hasFallback ? "fallback" : "missing";
}
