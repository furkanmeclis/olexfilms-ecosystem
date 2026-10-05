"use client";

import { useMemo, useState } from "react";

import { EditorX } from "@/components/editor/editor-x";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LOCALE_NAMES, type AppLocale } from "@/config/i18n";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { LexicalContractVariableMenu } from "@/features/contracts/components/contract-variable-menu";
import {
  useContractTemplate,
  useContractTemplateVariables,
  usePutContractTemplateLocale,
  useUpdateContractTemplate,
} from "@/features/contracts/hooks/use-contract-templates";
import {
  renderPreview,
  unknownVariables,
} from "@/features/contracts/lib/preview";
import { ContractVariablesExtension } from "@/features/contracts/lib/variable-node";
import {
  CONTRACT_TEMPLATE_LOCALES,
  findLocale,
  type ContractTemplate,
  type ContractTemplateLocale,
  type ContractTemplateVariable,
} from "@/features/contracts/services/contract-templates.service";
import { useLocale } from "@/providers/locale-provider";

const EDITOR_EXTENSIONS = [ContractVariablesExtension];

/** Contract template editor: settings, 13 language tabs, Lexical body. */
export function ContractTemplateEditor({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const query = useContractTemplate(uuid);

  return (
    <EntityPage
      title={query.data?.name ?? t("contract_templates.editor.title")}
      description={t("contract_templates.editor.description")}
      permission={permissions.contractTemplates.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("contract_templates.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("contract_templates.title"),
          href: routes.platform.contractTemplates.root,
        },
        { label: t("contract_templates.editor.title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("contract_templates.load_failed")}
        />
      ) : null}
      {query.data ? (
        <div className="grid gap-6">
          <SettingsCard
            key={`${query.data.uuid}:${query.data.updated_at}`}
            template={query.data}
          />
          <LocalesCard template={query.data} />
        </div>
      ) : null}
    </EntityPage>
  );
}

function SettingsCard({ template }: { template: ContractTemplate }) {
  const { t } = useLocale();
  const update = useUpdateContractTemplate(template.uuid);
  const [name, setName] = useState(template.name);
  const [isActive, setIsActive] = useState(template.is_active);
  const [otpRequired, setOtpRequired] = useState(template.otp_required);
  const [signatureRequired, setSignatureRequired] = useState(
    template.signature_required,
  );
  const dirty =
    name !== template.name ||
    isActive !== template.is_active ||
    otpRequired !== template.otp_required ||
    signatureRequired !== template.signature_required;

  const toggles = [
    {
      id: "contract-template-active",
      label: t("contract_templates.fields.is_active"),
      hint: null,
      value: isActive,
      set: setIsActive,
    },
    {
      id: "contract-template-otp",
      label: t("contract_templates.fields.otp_required"),
      hint: t("contract_templates.hints.otp_required"),
      value: otpRequired,
      set: setOtpRequired,
    },
    {
      id: "contract-template-signature",
      label: t("contract_templates.fields.signature_required"),
      hint: t("contract_templates.hints.signature_required"),
      value: signatureRequired,
      set: setSignatureRequired,
    },
  ];

  return (
    <section className="grid gap-4 rounded-lg border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="me-auto text-sm font-semibold">
          {t("contract_templates.editor.settings")}
        </h2>
        <Badge variant="secondary">
          {t(`contract_templates.kinds.${template.kind}`)}
        </Badge>
        {template.is_default ? (
          <Badge variant="success">
            {t("contract_templates.default_badge")}
          </Badge>
        ) : null}
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="contract-template-edit-name">
          {t("contract_templates.fields.name")}
        </Label>
        <Input
          id="contract-template-edit-name"
          value={name}
          maxLength={150}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        {toggles.map((row) => (
          <div key={row.id} className="flex items-start gap-3">
            <Switch
              id={row.id}
              checked={row.value}
              onCheckedChange={(v) => row.set(Boolean(v))}
            />
            <div className="grid gap-0.5">
              <Label htmlFor={row.id}>{row.label}</Label>
              {row.hint ? (
                <p className="text-muted-foreground text-xs">{row.hint}</p>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      <div>
        <Button
          onClick={() =>
            update.mutate({
              name: name.trim(),
              is_active: isActive,
              otp_required: otpRequired,
              signature_required: signatureRequired,
            })
          }
          disabled={!dirty || !name.trim() || update.isPending}
        >
          {t("contract_templates.actions.save")}
        </Button>
      </div>
    </section>
  );
}

function LocalesCard({ template }: { template: ContractTemplate }) {
  const { t } = useLocale();
  const variables = useContractTemplateVariables();
  const [locale, setLocale] = useState<AppLocale>("tr");
  const current = findLocale(template, locale);

  return (
    <section className="grid gap-4 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">
        {t("contract_templates.editor.content")}
      </h2>
      <Tabs value={locale} onValueChange={(v) => setLocale(v as AppLocale)}>
        <TabsList className="h-auto flex-wrap justify-start">
          {CONTRACT_TEMPLATE_LOCALES.map((code) => {
            const row = findLocale(template, code);
            return (
              <TabsTrigger
                key={code}
                value={code}
                data-missing={row ? undefined : "true"}
                title={
                  row
                    ? t("contract_templates.editor.version", {
                        version: row.version,
                      })
                    : t("contract_templates.editor.locale_missing")
                }
                className="gap-1.5"
              >
                {LOCALE_NAMES[code]}
                {row ? (
                  <span className="text-muted-foreground text-xs">
                    v{row.version}
                  </span>
                ) : (
                  <span
                    aria-label={t("contract_templates.editor.locale_missing")}
                    className="size-1.5 rounded-full bg-amber-500"
                  />
                )}
              </TabsTrigger>
            );
          })}
        </TabsList>
      </Tabs>
      <LocaleEditor
        key={`${locale}:${current?.version ?? 0}`}
        templateUuid={template.uuid}
        locale={locale}
        current={current}
        variables={variables.data?.items ?? []}
      />
    </section>
  );
}

function LocaleEditor({
  templateUuid,
  locale,
  current,
  variables,
}: {
  templateUuid: string;
  locale: AppLocale;
  current: ContractTemplateLocale | undefined;
  variables: readonly ContractTemplateVariable[];
}) {
  const { t, format } = useLocale();
  const save = usePutContractTemplateLocale(templateUuid);
  const initialState = useMemo(
    () => (current?.lexical_json ? JSON.stringify(current.lexical_json) : null),
    [current],
  );
  const [html, setHtml] = useState(current?.html ?? "");
  const [stateJson, setStateJson] = useState<string | null>(initialState);
  const [showPreview, setShowPreview] = useState(false);
  const unknown = unknownVariables(html, variables);
  const dirty = html !== (current?.html ?? "");
  const rtl = locale === "ar";

  const preview = useMemo(
    () =>
      showPreview
        ? renderPreview(html, variables, format.date(new Date()))
        : "",
    [showPreview, html, variables, format],
  );

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {current ? (
          <Badge variant="outline">
            {t("contract_templates.editor.version", {
              version: current.version,
            })}
          </Badge>
        ) : (
          <Badge variant="warning">
            {t("contract_templates.editor.locale_missing")}
          </Badge>
        )}
        {dirty ? (
          <span className="text-muted-foreground text-xs">
            {t("contract_templates.editor.unsaved")}
          </span>
        ) : null}
      </div>
      <div dir={rtl ? "rtl" : "ltr"}>
        <EditorX
          initialStateJson={initialState}
          initialHtml={current?.html ?? ""}
          placeholder={t("contract_templates.editor.placeholder")}
          extensions={EDITOR_EXTENSIONS}
          onHtmlChange={(nextHtml, json) => {
            setHtml(nextHtml);
            setStateJson(json);
          }}
          toolbarExtra={<LexicalContractVariableMenu variables={variables} />}
        />
      </div>
      {unknown.length > 0 ? (
        <p className="text-destructive text-sm">
          {t("contract_templates.editor.unknown_variables")}:{" "}
          {unknown.join(", ")}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={() =>
            save.mutate({
              locale,
              body: {
                html,
                lexical_json: stateJson
                  ? (JSON.parse(stateJson) as Record<string, unknown>)
                  : null,
              },
            })
          }
          disabled={
            !dirty || !html.trim() || unknown.length > 0 || save.isPending
          }
        >
          {t("contract_templates.actions.save_locale")}
        </Button>
        <Button
          variant="outline"
          onClick={() => setShowPreview((v) => !v)}
          disabled={!html.trim()}
        >
          {showPreview
            ? t("contract_templates.actions.hide_preview")
            : t("contract_templates.actions.preview")}
        </Button>
      </div>
      {showPreview ? (
        <div className="grid gap-1.5">
          <p className="text-muted-foreground text-xs">
            {t("contract_templates.editor.preview_hint")}
          </p>
          <iframe
            title={t("contract_templates.editor.preview")}
            sandbox=""
            srcDoc={`<!doctype html><html dir="${rtl ? "rtl" : "ltr"}" lang="${locale}"><head><meta charset="utf-8"></head><body style="font-family:system-ui,sans-serif;padding:16px">${preview}</body></html>`}
            className="h-[32rem] w-full rounded-lg border bg-white"
          />
        </div>
      ) : null}
    </div>
  );
}
