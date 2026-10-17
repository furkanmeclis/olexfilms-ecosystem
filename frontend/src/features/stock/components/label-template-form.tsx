"use client";

import { ImageUp } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { AppForm, AppInput, AppSelect, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LabelPreview } from "@/features/stock/components/label-preview";
import {
  KIND_DEFAULTS,
  LOGO_MODES,
  logoImageError,
  labelTemplateSchema,
  SYMBOLOGIES,
  TEMPLATE_KINDS,
  type LabelTemplateValues,
} from "@/features/stock/lib/label-templates";
import type { LabelTemplateKind } from "@/features/stock/services/label-templates.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/**
 * Create / edit dialog of a label template (TEC-764) with a live preview
 * of the label next to the fields. The kind is fixed after create; a
 * location label is always a QR code.
 */
export function LabelTemplateFormDialog({
  mode,
  initial,
  pending,
  onSubmit,
  onClose,
}: {
  mode: "create" | "edit";
  initial: LabelTemplateValues;
  pending: boolean;
  onSubmit: (values: LabelTemplateValues) => Promise<unknown>;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [serverError, setServerError] = useState<string | null>(null);
  const formId = useId();

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="max-h-[92vh] overflow-y-auto sm:max-w-4xl"
        data-testid="label-template-form"
        data-mode={mode}
      >
        <DialogHeader>
          <DialogTitle>
            {t(`stock.label_templates.form.${mode}_title`)}
          </DialogTitle>
          <DialogDescription>
            {t("stock.label_templates.form.description")}
          </DialogDescription>
        </DialogHeader>
        <AppForm<LabelTemplateValues>
          id={formId}
          schema={labelTemplateSchema(t) as never}
          defaultValues={initial}
          onSubmit={async (values) => {
            setServerError(null);
            try {
              await onSubmit(values);
            } catch (err) {
              setServerError(templateErrorMessage(err, t));
            }
          }}
          className="grid gap-6 lg:grid-cols-[1fr_minmax(0,1.1fr)]"
        >
          <TemplateFields mode={mode} />
          <div className="space-y-2 lg:sticky lg:top-0 lg:self-start">
            <p className="text-sm font-medium">
              {t("stock.label_templates.preview.title")}
            </p>
            <WatchedPreview />
            {serverError ? (
              <p
                role="alert"
                className="text-destructive text-sm"
                data-testid="label-template-error"
              >
                {serverError}
              </p>
            ) : null}
          </div>
        </AppForm>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            disabled={pending}
            data-testid="label-template-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function templateErrorMessage(
  err: unknown,
  t: (key: string) => string,
): string {
  if (isApiError(err)) {
    const key = `stock.label_templates.errors.${err.code}`;
    const text = t(key);
    if (text !== key) return text;
  }
  return t("stock.label_templates.errors.generic");
}

function WatchedPreview() {
  const values = useWatch() as LabelTemplateValues;
  return <LabelPreview values={values} />;
}

function TemplateFields({ mode }: { mode: "create" | "edit" }) {
  const { t } = useLocale();
  const { setValue, control } = useFormContext<LabelTemplateValues>();
  const kind = useWatch({ control, name: "kind" });
  const logoMode = useWatch({ control, name: "logo_mode" });

  // A new template of another kind starts from that kind's defaults.
  const lastKind = useRef<LabelTemplateKind>(kind);
  useEffect(() => {
    if (mode !== "create" || lastKind.current === kind) return;
    lastKind.current = kind;
    const d = KIND_DEFAULTS[kind];
    setValue("symbology", d.symbology);
    setValue("width_mm", d.width_mm);
    setValue("height_mm", d.height_mm);
    setValue("columns", d.columns);
  }, [kind, mode, setValue]);

  return (
    <div className="space-y-4">
      <AppInput
        name="name"
        label={t("stock.label_templates.fields.name")}
        autoComplete="off"
        data-testid="label-template-name"
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="kind"
          label={t("stock.label_templates.fields.kind")}
          description={
            mode === "edit"
              ? t("stock.label_templates.form.kind_fixed")
              : undefined
          }
          disabled={mode === "edit"}
          options={TEMPLATE_KINDS.map((k) => ({
            value: k,
            label: t(`stock.label_templates.kind.${k}`),
          }))}
          testId="label-template-kind"
        />
        <AppSelect
          name="symbology"
          label={t("stock.label_templates.fields.symbology")}
          description={
            kind === "location"
              ? t("stock.label_templates.form.location_qr")
              : undefined
          }
          disabled={kind === "location"}
          options={SYMBOLOGIES.map((s) => ({
            value: s,
            label: t(`stock.label_templates.symbology.${s}`),
          }))}
          testId="label-template-symbology"
        />
      </div>
      <div className="grid grid-cols-3 gap-4">
        <AppInput
          name="width_mm"
          label={t("stock.label_templates.fields.width_mm")}
          inputMode="decimal"
          dir="ltr"
          data-testid="label-template-width"
        />
        <AppInput
          name="height_mm"
          label={t("stock.label_templates.fields.height_mm")}
          inputMode="decimal"
          dir="ltr"
          data-testid="label-template-height"
        />
        <AppInput
          name="columns"
          label={t("stock.label_templates.fields.columns")}
          inputMode="numeric"
          dir="ltr"
          data-testid="label-template-columns"
        />
      </div>
      <AppSelect
        name="logo_mode"
        label={t("stock.label_templates.fields.logo_mode")}
        options={LOGO_MODES.map((m) => ({
          value: m,
          label: t(`stock.label_templates.logo_mode.${m}`),
        }))}
        testId="label-template-logo-mode"
      />
      {logoMode === "text" ? (
        <AppInput
          name="logo_text"
          label={t("stock.label_templates.fields.logo_text")}
          data-testid="label-template-logo-text"
        />
      ) : null}
      {logoMode === "image" ? <LogoUpload /> : null}
      <AppSwitch
        name="show_code_text"
        label={t("stock.label_templates.fields.show_code_text")}
      />
      <AppSwitch
        name="show_name"
        label={t("stock.label_templates.fields.show_name")}
        description={t("stock.label_templates.form.show_name_hint")}
      />
      <AppSwitch
        name="is_default"
        label={t("stock.label_templates.fields.is_default")}
        description={t("stock.label_templates.form.default_hint")}
      />
      <AppSwitch
        name="active"
        label={t("stock.label_templates.fields.active")}
      />
    </div>
  );
}

/** PNG / JPEG logo read as a data URI (validated like the backend). */
function LogoUpload() {
  const { t } = useLocale();
  const id = useId();
  const {
    setValue,
    control,
    formState: { errors },
  } = useFormContext<LabelTemplateValues>();
  const image = useWatch({ control, name: "logo_image" });
  const [localError, setLocalError] = useState<string | null>(null);
  const error =
    localError ?? (errors.logo_image?.message as string | undefined);

  const onFile = (file: File | undefined) => {
    setLocalError(null);
    if (!file) return;
    if (file.type !== "image/png" && file.type !== "image/jpeg") {
      setLocalError(t("stock.label_templates.validation.logo_type"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const uri = String(reader.result ?? "");
      const problem = logoImageError(uri);
      if (problem) {
        setLocalError(t(`stock.label_templates.validation.${problem}`));
        return;
      }
      setValue("logo_image", uri, { shouldValidate: true, shouldDirty: true });
    };
    reader.readAsDataURL(file);
  };

  return (
    <div className="space-y-2">
      <label htmlFor={id} className="text-sm font-medium">
        {t("stock.label_templates.fields.logo_image")}
      </label>
      <div className="flex items-center gap-3">
        {image ? (
          // eslint-disable-next-line @next/next/no-img-element -- data URI
          <img
            src={image}
            alt=""
            className="h-10 max-w-32 rounded border object-contain"
          />
        ) : null}
        <Button type="button" variant="outline" size="sm" asChild>
          <label htmlFor={id} className="cursor-pointer">
            <ImageUp className="size-4" />
            {t("stock.label_templates.form.logo_pick")}
          </label>
        </Button>
        <input
          id={id}
          type="file"
          accept="image/png,image/jpeg"
          className="sr-only"
          data-testid="label-template-logo-file"
          onChange={(e) => onFile(e.target.files?.[0])}
        />
      </div>
      <p className="text-muted-foreground text-xs">
        {t("stock.label_templates.form.logo_hint")}
      </p>
      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : null}
    </div>
  );
}
