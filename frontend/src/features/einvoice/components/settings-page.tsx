"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Upload } from "lucide-react";
import { useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppSwitch, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  EinvoiceShell,
  useEinvoiceAccess,
} from "@/features/einvoice/components/einvoice-shell";
import {
  settingsBody,
  settingsDefaults,
  settingsFormSchema,
  type SettingsFormValues,
} from "@/features/einvoice/lib/settings-form";
import {
  einvoiceKeys,
  einvoiceService,
  type EinvoiceSettings,
} from "@/features/einvoice/services/einvoice.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const S = "einvoice.settings";
const XSLT_MAX_BYTES = 1 << 20;

function SettingsForm({
  settings,
  editable,
}: {
  settings: EinvoiceSettings;
  editable: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: (values: SettingsFormValues) =>
      einvoiceService.putSettings(settingsBody(values)),
    onSuccess: (updated) => {
      qc.setQueryData(einvoiceKeys.settings, updated);
      appToast.success(t(`${S}.saved`));
    },
  });
  const field = (name: keyof SettingsFormValues) => ({
    name,
    label: t(`${S}.fields.${name}`),
    disabled: !editable,
  });

  return (
    <AppForm<SettingsFormValues>
      schema={settingsFormSchema(t)}
      defaultValues={settingsDefaults(settings)}
      onSubmit={async (values) => {
        await save.mutateAsync(values);
      }}
      className="space-y-6"
    >
      <EntitySectionCard title={t(`${S}.seller`)}>
        <div className="grid gap-4 md:grid-cols-2">
          <AppInput {...field("legal_name")} />
          <AppInput {...field("vkn")} inputMode="numeric" dir="ltr" />
          <AppInput {...field("tax_office")} />
          <AppInput {...field("mersis_no")} inputMode="numeric" dir="ltr" />
          <AppTextarea
            {...field("address")}
            rows={2}
            className="md:col-span-2"
          />
          <AppInput {...field("district")} />
          <AppInput {...field("city")} />
          <AppInput {...field("trade_registry_no")} />
          <AppInput {...field("iban")} dir="ltr" />
          <AppInput {...field("email")} type="email" dir="ltr" />
          <AppInput
            {...field("phone")}
            type="tel"
            dir="ltr"
            placeholder="+905551112233"
          />
          <AppInput {...field("website")} dir="ltr" placeholder="https://" />
          <AppTextarea
            {...field("default_note")}
            rows={2}
            className="md:col-span-2"
          />
        </div>
      </EntitySectionCard>

      <EntitySectionCard title={t(`${S}.series`)}>
        <p className="text-muted-foreground mb-4 text-sm">
          {t(`${S}.series_hint`)}
        </p>
        <div className="grid gap-4 md:grid-cols-2">
          <AppInput
            {...field("earchive_series")}
            description={t(`${S}.earchive_series_hint`)}
            maxLength={3}
            className="font-mono uppercase"
            dir="ltr"
            data-testid="einvoice-earchive-series"
          />
          <AppInput
            {...field("efatura_series")}
            description={t(`${S}.efatura_series_hint`)}
            maxLength={3}
            className="font-mono uppercase"
            dir="ltr"
            data-testid="einvoice-efatura-series"
          />
          <AppSwitch
            name="pdf_enabled"
            label={t(`${S}.fields.pdf_enabled`)}
            description={t(`${S}.pdf_enabled_hint`)}
            disabled={!editable}
          />
        </div>
      </EntitySectionCard>

      {editable ? (
        <div className="flex justify-end">
          <Button
            type="submit"
            disabled={save.isPending}
            data-testid="einvoice-settings-save"
          >
            {t("common.save")}
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">{t(`${S}.read_only`)}</p>
      )}
    </AppForm>
  );
}

/** Year × series last numbers, read only (TEC-504). */
function CountersCard({ settings }: { settings: EinvoiceSettings }) {
  const { t, format } = useLocale();
  const counters = settings.counters ?? [];
  return (
    <EntitySectionCard title={t(`${S}.counters`)}>
      {counters.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t(`${S}.counters_empty`)}
        </p>
      ) : (
        <dl
          className="grid gap-3 sm:grid-cols-2"
          data-testid="einvoice-counters"
        >
          {counters.map((c) => (
            <div
              key={`${c.year}-${c.series}`}
              className="rounded-md border p-3"
            >
              <dt className="text-muted-foreground text-xs">
                {t(`${S}.counter_label`, { year: c.year, series: c.series })}
              </dt>
              <dd className="font-mono text-sm" dir="ltr">
                {c.series}
                {c.year}
                {String(c.last_no).padStart(9, "0")}
              </dd>
              {c.updated_at ? (
                <dd className="text-muted-foreground text-xs">
                  {format.dateTime(c.updated_at)}
                </dd>
              ) : null}
            </div>
          ))}
        </dl>
      )}
    </EntitySectionCard>
  );
}

/** XSLT upload / reset and a sample invoice rendered with it. */
function StylesheetCard({
  settings,
  editable,
}: {
  settings: EinvoiceSettings;
  editable: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [confirmReset, setConfirmReset] = useState(false);
  const sample = useQuery({
    queryKey: einvoiceKeys.samplePreview(settings.xslt_sha1 ?? "default"),
    queryFn: () => einvoiceService.samplePreview(),
    enabled: settings.configured,
  });
  const onSaved = (updated: EinvoiceSettings) => {
    qc.setQueryData(einvoiceKeys.settings, updated);
    void qc.invalidateQueries({ queryKey: ["einvoices", "sample"] });
  };
  const upload = useMutation({
    mutationFn: (file: File) => einvoiceService.uploadXslt(file),
    onSuccess: (updated) => {
      onSaved(updated);
      appToast.success(t(`${S}.xslt_uploaded`));
    },
  });
  const reset = useMutation({
    mutationFn: () => einvoiceService.resetXslt(),
    onSuccess: (updated) => {
      setConfirmReset(false);
      onSaved(updated);
      appToast.success(t(`${S}.xslt_reset_done`));
    },
    onError: () => setConfirmReset(false),
  });
  const pick = (file: File | undefined) => {
    if (!file) return;
    if (file.size > XSLT_MAX_BYTES) {
      appToast.error(t(`${S}.xslt_too_large`));
      return;
    }
    upload.mutate(file);
  };

  return (
    <EntitySectionCard
      title={t(`${S}.xslt`)}
      action={
        <StatusChip
          label={
            settings.custom_xslt
              ? t(`${S}.xslt_custom`)
              : t(`${S}.xslt_default`)
          }
          tone={settings.custom_xslt ? "success" : "default"}
        />
      }
    >
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm">{t(`${S}.xslt_hint`)}</p>
        {settings.xslt_sha1 ? (
          <p className="text-muted-foreground font-mono text-xs" dir="ltr">
            SHA-1 {settings.xslt_sha1}
          </p>
        ) : null}
        {editable ? (
          <div className="flex flex-wrap gap-2">
            <input
              ref={input}
              type="file"
              accept=".xsl,.xslt,.xml,application/xml,text/xml"
              className="hidden"
              data-testid="einvoice-xslt-input"
              onChange={(e) => {
                pick(e.target.files?.[0]);
                e.target.value = "";
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!settings.configured || upload.isPending}
              title={
                settings.configured ? undefined : t(`${S}.xslt_needs_settings`)
              }
              onClick={() => input.current?.click()}
            >
              <Upload className="size-4" />
              {t(`${S}.xslt_upload`)}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!settings.custom_xslt || reset.isPending}
              onClick={() => setConfirmReset(true)}
            >
              <RotateCcw className="size-4" />
              {t(`${S}.xslt_reset`)}
            </Button>
          </div>
        ) : null}
        {!settings.configured ? (
          <p className="text-muted-foreground text-sm">
            {t(`${S}.xslt_needs_settings`)}
          </p>
        ) : sample.isLoading ? (
          <Loading />
        ) : sample.isError || sample.data === undefined ? (
          <ErrorState
            title={t("einvoice.detail.preview_error")}
            description={
              isApiError(sample.error)
                ? t(`errors.codes.${sample.error.code}`)
                : ""
            }
            onRetry={() => void sample.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <iframe
            title={t(`${S}.xslt_sample`)}
            sandbox=""
            srcDoc={sample.data}
            data-testid="einvoice-xslt-sample"
            className="h-[min(70vh,720px)] w-full rounded-md border bg-white"
          />
        )}
      </div>
      <ConfirmDialog
        open={confirmReset}
        title={t(`${S}.xslt_reset_title`)}
        description={t(`${S}.xslt_reset_description`)}
        confirmLabel={t(`${S}.xslt_reset`)}
        isPending={reset.isPending}
        onConfirm={() => reset.mutate()}
        onCancel={() => setConfirmReset(false)}
      />
    </EntitySectionCard>
  );
}

function SettingsBody({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { canSettings } = useEinvoiceAccess(slug);
  const query = useQuery({
    queryKey: einvoiceKeys.settings,
    queryFn: () => einvoiceService.getSettings(),
  });
  if (query.isLoading) return <Loading />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const settings = query.data;
  return (
    <div className="space-y-6">
      {!settings.configured ? (
        <p
          className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
          data-testid="einvoice-not-configured"
        >
          {t(`${S}.not_configured`)}
        </p>
      ) : null}
      <SettingsForm
        key={settings.updated_at ?? "new"}
        settings={settings}
        editable={canSettings}
      />
      <CountersCard settings={settings} />
      <StylesheetCard settings={settings} editable={canSettings} />
    </div>
  );
}

/**
 * Muhasebe > e-Fatura > Ayarlar (TEC-504): seller profile, the e-Arşiv /
 * e-Fatura series (3 characters), the read-only counter state per year and
 * series, the XSLT (upload / back to the GİB default) with a sample
 * preview. Writing needs einvoice.settings.
 */
export function EinvoiceSettingsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <EinvoiceShell
      slug={slug}
      section="settings"
      title={t(`${S}.title`)}
      description={t(`${S}.description`)}
    >
      <SettingsBody slug={slug} />
    </EinvoiceShell>
  );
}
