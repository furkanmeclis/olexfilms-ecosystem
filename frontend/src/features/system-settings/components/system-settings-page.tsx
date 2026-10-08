"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Blocks,
  BellRing,
  BadgeCheck,
  Coins,
  FileText,
  KeyRound,
  MapPinned,
  MessageCircle,
  RectangleHorizontal,
  RotateCcw,
  Save,
  ScrollText,
  Settings2,
  SlidersHorizontal,
  Warehouse,
} from "lucide-react";
import Link from "next/link";
import { useState, type ComponentType } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  groupSettings,
  parseDraft,
  SECRET_MASK,
  toDraft,
  type Draft,
  type DraftError,
} from "@/features/system-settings/lib/setting-value";
import { splitLaborSettings } from "@/features/system-settings/lib/labor-rule";
import { WarrantyLaborRuleCard } from "@/features/system-settings/components/warranty-labor-rule-card";
import {
  systemSettingsService,
  type SystemSetting,
  type SystemSettingValue,
} from "@/features/system-settings/services/system-settings.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export const systemSettingsKeys = {
  all: ["system-settings"] as const,
};

type HubLink = {
  id: string;
  href: string;
  titleKey: string;
  descriptionKey: string;
  icon: ComponentType<{ className?: string }>;
  permission: string;
};

/** Existing topic pages reachable from the hub (shown only when permitted). */
export const HUB_LINKS: HubLink[] = [
  {
    id: "export-settings",
    href: routes.platform.settings.root,
    titleKey: "layout.nav_export_settings",
    descriptionKey: "settings.system.hub.export_settings",
    icon: Settings2,
    permission: permissions.settings.read,
  },
  {
    id: "territories",
    href: routes.platform.territories.root,
    titleKey: "layout.nav_territories",
    descriptionKey: "settings.system.hub.territories",
    icon: MapPinned,
    permission: permissions.territories.read,
  },
  {
    id: "plate-formats",
    href: routes.platform.plateFormats.root,
    titleKey: "layout.nav_plate_formats",
    descriptionKey: "settings.system.hub.plate_formats",
    icon: RectangleHorizontal,
    permission: permissions.settings.read,
  },
  {
    id: "exchange-rates",
    href: routes.platform.exchangeRates.root,
    titleKey: "layout.nav_exchange_rates",
    descriptionKey: "settings.system.hub.exchange_rates",
    icon: Coins,
    permission: permissions.rates.read,
  },
  {
    id: "legal-texts",
    href: routes.platform.legalTexts,
    titleKey: "layout.nav_legal_texts",
    descriptionKey: "settings.system.hub.legal_texts",
    icon: ScrollText,
    permission: permissions.legalTexts.write,
  },
  {
    id: "modules",
    href: routes.platform.modules.root,
    titleKey: "layout.nav_modules",
    descriptionKey: "settings.system.hub.modules",
    icon: Blocks,
    permission: permissions.modules.platformRead,
  },
  {
    id: "whatsapp",
    href: routes.platform.integrations.whatsapp,
    titleKey: "layout.nav_whatsapp_integration",
    descriptionKey: "settings.system.hub.whatsapp",
    icon: MessageCircle,
    permission: permissions.integrations.whatsapp.manage,
  },
  {
    id: "glorian",
    href: routes.platform.integrations.glorian,
    titleKey: "layout.nav_glorian_integration",
    descriptionKey: "settings.system.hub.glorian",
    icon: Warehouse,
    permission: permissions.integrations.glorian.view,
  },
  {
    id: "notification-center",
    href: routes.platform.notifications.center,
    titleKey: "layout.nav_notification_center",
    descriptionKey: "settings.system.hub.notification_center",
    icon: BellRing,
    permission: permissions.notifications.templatesManage,
  },
  {
    id: "auth-settings",
    href: routes.platform.authSettings.root,
    titleKey: "layout.nav_auth_settings",
    descriptionKey: "settings.system.hub.auth_settings",
    icon: KeyRound,
    permission: permissions.authSettings.read,
  },
  {
    id: "document-templates",
    href: routes.platform.documentTemplates.root,
    titleKey: "layout.nav_document_templates",
    descriptionKey: "settings.system.hub.document_templates",
    icon: FileText,
    permission: permissions.documentTemplates.read,
  },
  {
    id: "certificate-types",
    href: routes.platform.certificateTypes.root,
    titleKey: "certificates.types.nav",
    descriptionKey: "settings.system.hub.certificates",
    icon: BadgeCheck,
    permission: permissions.certificates.typesManage,
  },
];

/** Translated text, or the fallback when the key has no catalog entry. */
function tOr(t: Translate, key: string, fallback: string) {
  const text = t(key);
  return text === key ? fallback : text;
}

function settingSlug(key: string) {
  return key.replace(/[^a-z0-9]+/gi, "_");
}

function draftErrorText(t: Translate, error: DraftError) {
  switch (error.code) {
    case "integer":
      return t("settings.system.validation.integer");
    case "min":
      return t("settings.system.validation.min", { min: error.min });
    case "max":
      return t("settings.system.validation.max", { max: error.max });
    case "max_len":
      return t("settings.system.validation.max_len", { max: error.max });
  }
}

function formatValue(value: SystemSettingValue | undefined, t: Translate) {
  if (typeof value === "boolean") {
    return value ? t("settings.system.on") : t("settings.system.off");
  }
  if (value === "" || value === undefined) return t("settings.system.empty");
  return String(value);
}

/** Admin > System settings: hub links + the TEC-215 key/value catalog. */
export function SystemSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.settings.read);
  const canWrite = can(permissions.settings.write);

  const list = useQuery({
    queryKey: systemSettingsKeys.all,
    queryFn: () => systemSettingsService.list(),
    enabled: canRead,
  });

  const links = HUB_LINKS.filter((link) => can(link.permission));
  // TEC-340: the warranty labor rule keys get their own card (gated on the
  // catalog shipping them); every other key keeps the generic row.
  const { labor, rest } = splitLaborSettings(list.data?.items ?? []);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.system.title")}
        description={t("settings.system.description")}
        icon={<SlidersHorizontal className="size-6" />}
      />

      {links.length ? (
        <section className="space-y-3" aria-labelledby="system-settings-hub">
          <h2 id="system-settings-hub" className="text-lg font-semibold">
            {t("settings.system.hub.title")}
          </h2>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {links.map((link) => {
              const Icon = link.icon;
              return (
                <Link
                  key={link.id}
                  href={link.href}
                  data-testid={`hub-link-${link.id}`}
                  className="hover:bg-muted/50 focus-visible:ring-ring flex items-start gap-3 rounded-lg border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
                >
                  <Icon className="text-muted-foreground mt-0.5 size-5 shrink-0" />
                  <span className="min-w-0 space-y-1">
                    <span className="block font-medium">
                      {t(link.titleKey)}
                    </span>
                    <span className="text-muted-foreground block text-sm">
                      {t(link.descriptionKey)}
                    </span>
                  </span>
                </Link>
              );
            })}
          </div>
        </section>
      ) : null}

      {!canRead ? null : list.isLoading ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState
          title={t("settings.system.load_failed")}
          onRetry={() => void list.refetch()}
        />
      ) : (
        <div className="space-y-4">
          {!canWrite ? (
            <p className="text-muted-foreground text-sm">
              {t("settings.system.read_only")}
            </p>
          ) : null}
          {labor ? (
            <WarrantyLaborRuleCard
              key={labor.rule.updated_at ?? ""}
              labor={labor}
              canWrite={canWrite}
              queryKey={systemSettingsKeys.all}
            />
          ) : null}
          {groupSettings(rest).map(({ group, settings }) => (
            <Card key={group} data-testid={`group-${group}`}>
              <CardHeader>
                <CardTitle>
                  {tOr(t, `settings.system.groups.${group}`, group)}
                </CardTitle>
                <CardDescription>
                  {tOr(t, `settings.system.groups.${group}_hint`, "")}
                </CardDescription>
              </CardHeader>
              <CardContent className="divide-y">
                {settings.map((setting) => (
                  <SettingRow
                    // Re-mount on server change so the draft follows the value.
                    key={`${setting.key}:${setting.updated_at ?? ""}:${String(setting.value)}`}
                    setting={setting}
                    canWrite={canWrite}
                  />
                ))}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function SettingRow({
  setting,
  canWrite,
}: {
  setting: SystemSetting;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft>(() => toDraft(setting));
  const [serverError, setServerError] = useState<string | null>(null);
  const slug = settingSlug(setting.key);
  const inputId = `setting-${slug}`;
  const label = tOr(t, `settings.system.keys.${slug}`, setting.key);
  const hint = tOr(t, `settings.system.keys.${slug}_hint`, setting.description);

  const parsed = parseDraft(setting, draft);
  const clientError =
    "error" in parsed ? draftErrorText(t, parsed.error) : null;
  const dirty = setting.secret ? draft !== "" : draft !== toDraft(setting);
  const error = clientError ?? serverError;

  const onSaved = (next: SystemSetting, message: string) => {
    queryClient.setQueryData<{ items: SystemSetting[] }>(
      systemSettingsKeys.all,
      (old) =>
        old
          ? {
              ...old,
              items: old.items.map((s) => (s.key === next.key ? next : s)),
            }
          : old,
    );
    setDraft(toDraft(next));
    setServerError(null);
    appToast.success(message);
  };
  const onError = (err: unknown) => {
    if (isApiError(err) && err.isValidation) {
      const fields = err.fieldErrors();
      setServerError(fields.value ?? err.message);
      return;
    }
    appToast.error(
      isApiError(err) ? err.message : t("settings.system.toast.failed"),
    );
  };

  const save = useMutation({
    mutationFn: (value: SystemSettingValue) =>
      systemSettingsService.put(setting.key, value),
    onSuccess: (next) => onSaved(next, t("settings.system.toast.saved")),
    onError,
  });
  const reset = useMutation({
    mutationFn: () => systemSettingsService.reset(setting.key),
    onSuccess: (next) => onSaved(next, t("settings.system.toast.reset")),
    onError,
  });
  const busy = save.isPending || reset.isPending;

  const submit = () => {
    if ("value" in parsed) save.mutate(parsed.value);
  };

  return (
    <div
      className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 lg:flex-row lg:items-start lg:justify-between"
      data-testid={`setting-${setting.key}`}
    >
      <div className="min-w-0 space-y-1 lg:max-w-md">
        <div className="flex flex-wrap items-center gap-2">
          <Label htmlFor={inputId}>{label}</Label>
          {setting.is_default ? (
            <Badge variant="outline">
              {t("settings.system.default_badge")}
            </Badge>
          ) : (
            <Badge variant="secondary">
              {t("settings.system.custom_badge")}
            </Badge>
          )}
        </div>
        <p className="text-muted-foreground text-sm">{hint}</p>
        <p className="text-muted-foreground text-xs">
          {t("settings.system.default_value", {
            value: setting.secret
              ? t("settings.system.empty")
              : formatValue(setting.default, t),
          })}
          {setting.kind === "int" &&
          setting.min !== undefined &&
          setting.max !== undefined
            ? ` · ${t("settings.system.range", { min: setting.min, max: setting.max })}`
            : null}
        </p>
      </div>

      <form
        className="flex w-full flex-col gap-2 lg:w-96"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        {setting.kind === "bool" ? (
          <Switch
            id={inputId}
            checked={draft === true}
            disabled={!canWrite || busy}
            onCheckedChange={(checked) => {
              setServerError(null);
              setDraft(checked);
            }}
          />
        ) : (
          <Input
            id={inputId}
            name={setting.key}
            type={
              setting.secret
                ? "password"
                : setting.kind === "int"
                  ? "number"
                  : "text"
            }
            inputMode={setting.kind === "int" ? "numeric" : undefined}
            min={setting.kind === "int" ? setting.min : undefined}
            max={setting.kind === "int" ? setting.max : undefined}
            maxLength={setting.max_len || undefined}
            autoComplete={setting.secret ? "new-password" : "off"}
            placeholder={
              setting.secret && setting.value === SECRET_MASK
                ? SECRET_MASK
                : undefined
            }
            value={typeof draft === "string" ? draft : ""}
            disabled={!canWrite || busy}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? `${inputId}-error` : undefined}
            onChange={(event) => {
              setServerError(null);
              setDraft(event.target.value);
            }}
          />
        )}
        {setting.secret ? (
          <p className="text-muted-foreground text-xs">
            {t("settings.system.secret_hint")}
          </p>
        ) : null}
        {error ? (
          <p
            id={`${inputId}-error`}
            role="alert"
            className="text-destructive text-sm"
          >
            {error}
          </p>
        ) : null}
        {canWrite ? (
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={busy || setting.is_default}
              onClick={() => reset.mutate()}
            >
              <RotateCcw className="size-4" />
              {t("settings.system.reset")}
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={busy || !dirty || clientError !== null}
            >
              <Save className="size-4" />
              {t("settings.system.save")}
            </Button>
          </div>
        ) : null}
      </form>
    </div>
  );
}
