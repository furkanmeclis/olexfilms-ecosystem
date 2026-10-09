"use client";

import { ImageIcon } from "lucide-react";
import { useState } from "react";
import { z } from "zod";

import { AppForm, AppInput, AppSwitch, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import {
  MAX_PHOTO_BYTES,
  validatePhotoFile,
} from "@/features/photo-standard/lib/photo-standard";
import {
  photoSrc,
  type PhotoAngle,
  type PhotoAngleInput,
} from "@/features/photo-standard/services/photo-standard.service";
import { useLocale } from "@/providers/locale-provider";

const T = "photo_standard.angles";
const KEY_PATTERN = /^[a-z0-9][a-z0-9_-]{1,62}[a-z0-9]$/;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Form field of a language tab ("zh-CN" → "name_zh_CN"). */
export const localeField = (prefix: "name" | "hint", locale: AppLocale) =>
  `${prefix}_${locale.replace("-", "_")}`;

export type PhotoAngleFormValues = {
  key: string;
  required: boolean;
  active: boolean;
} & Record<string, string | boolean>;

function angleSchema(t: Translate) {
  const texts = Object.fromEntries(
    SUPPORTED_LOCALES.flatMap((locale) => [
      [
        localeField("name", locale),
        locale === "tr"
          ? z
              .string()
              .trim()
              .min(1, t(`${T}.validation.name_tr`))
              .max(120)
          : z.string().trim().max(120),
      ],
      [localeField("hint", locale), z.string().trim().max(500)],
    ]),
  );
  return z.object({
    key: z
      .string()
      .trim()
      .regex(KEY_PATTERN, t(`${T}.validation.key`)),
    required: z.boolean(),
    active: z.boolean(),
    ...texts,
  });
}

function defaults(angle: PhotoAngle | null): PhotoAngleFormValues {
  return {
    key: angle?.key ?? "",
    required: angle?.required ?? true,
    active: angle?.active ?? true,
    ...Object.fromEntries(
      SUPPORTED_LOCALES.flatMap((locale) => [
        [localeField("name", locale), angle?.name?.[locale] ?? ""],
        [localeField("hint", locale), angle?.hint?.[locale] ?? ""],
      ]),
    ),
  };
}

/** Builds the API body: empty language texts are left out. */
export function angleBody(
  values: PhotoAngleFormValues,
  base: { sort_order: number; example_storage_key: string | null },
): PhotoAngleInput {
  const pick = (prefix: "name" | "hint") =>
    Object.fromEntries(
      SUPPORTED_LOCALES.map((locale) => [
        locale,
        String(values[localeField(prefix, locale)] ?? "").trim(),
      ]).filter(([, text]) => text !== ""),
    );
  return {
    key: values.key.trim(),
    name: pick("name"),
    hint: pick("hint"),
    required: values.required,
    active: values.active,
    sort_order: base.sort_order,
    example_storage_key: base.example_storage_key,
  };
}

export type PhotoAngleSubmit = {
  values: PhotoAngleFormValues;
  example: File | null;
  removeExample: boolean;
};

/**
 * Create / edit a central intake angle (TEC-500): key (fixed after
 * creation), required / active, the name and the hint in 13 language tabs
 * (Turkish name required) and the example image shown on the wizard cards.
 */
export function PhotoAngleDialog({
  open,
  angle,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  angle: PhotoAngle | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (submit: PhotoAngleSubmit) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [example, setExample] = useState<File | null>(null);
  const [exampleError, setExampleError] = useState<string | null>(null);
  const [removeExample, setRemoveExample] = useState(false);
  const schema = angleSchema(t) as unknown as z.ZodType<
    PhotoAngleFormValues,
    PhotoAngleFormValues
  >;

  const change = (next: boolean) => {
    if (!next) {
      setExample(null);
      setExampleError(null);
      setRemoveExample(false);
    }
    onOpenChange(next);
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {angle ? t(`${T}.edit_title`) : t(`${T}.create_title`)}
          </DialogTitle>
          <DialogDescription>{t(`${T}.form_description`)}</DialogDescription>
        </DialogHeader>
        {open ? (
          <AppForm<PhotoAngleFormValues>
            key={angle?.uuid ?? "new"}
            schema={schema}
            defaultValues={defaults(angle)}
            onSubmit={async (values) => {
              if (exampleError) return;
              await onSubmit({ values, example, removeExample });
              setExample(null);
              setRemoveExample(false);
            }}
            className="space-y-4"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="key"
                label={t(`${T}.fields.key`)}
                description={t(`${T}.key_hint`)}
                readOnly={Boolean(angle)}
                dir="ltr"
              />
              <div className="flex flex-col justify-end gap-3">
                <AppSwitch name="required" label={t(`${T}.fields.required`)} />
                <AppSwitch name="active" label={t(`${T}.fields.active`)} />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="angle-example">{t(`${T}.fields.example`)}</Label>
              <div className="flex items-start gap-3">
                <div className="bg-muted flex size-20 shrink-0 items-center justify-center overflow-hidden rounded-md border">
                  {angle?.example_url && !removeExample && !example ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={photoSrc(angle.example_url)}
                      alt={t(`${T}.fields.example`)}
                      className="size-full object-contain"
                    />
                  ) : (
                    <ImageIcon className="text-muted-foreground size-6" />
                  )}
                </div>
                <div className="flex-1 space-y-2">
                  <Input
                    id="angle-example"
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    data-testid="angle-example-input"
                    onChange={(e) => {
                      const file = e.target.files?.[0] ?? null;
                      const invalid = file ? validatePhotoFile(file) : null;
                      setExampleError(
                        invalid
                          ? t(`photo_standard.intake.errors.${invalid}`)
                          : null,
                      );
                      setExample(invalid ? null : file);
                    }}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t(`${T}.example_hint`, {
                      mb: MAX_PHOTO_BYTES / 1024 / 1024,
                    })}
                  </p>
                  {exampleError ? (
                    <p className="text-destructive text-xs" role="alert">
                      {exampleError}
                    </p>
                  ) : null}
                  {angle?.example_url ? (
                    <Label className="flex items-center gap-2 text-sm font-normal">
                      <Checkbox
                        checked={removeExample}
                        onCheckedChange={(v) => setRemoveExample(v === true)}
                      />
                      {t(`${T}.remove_example`)}
                    </Label>
                  ) : null}
                </div>
              </div>
            </div>
            <Tabs defaultValue="tr">
              <TabsList className="h-auto flex-wrap justify-start">
                {SUPPORTED_LOCALES.map((locale) => (
                  <TabsTrigger
                    key={locale}
                    value={locale}
                    data-locale-tab={locale}
                  >
                    {LOCALE_NAMES[locale]}
                  </TabsTrigger>
                ))}
              </TabsList>
              {SUPPORTED_LOCALES.map((locale) => (
                <TabsContent
                  key={locale}
                  value={locale}
                  forceMount
                  className="space-y-3 data-[state=inactive]:hidden"
                >
                  <AppInput
                    name={localeField("name", locale)}
                    label={t(`${T}.fields.name`)}
                    description={
                      locale === "tr" ? t(`${T}.name_tr_hint`) : undefined
                    }
                    dir={localeDir(locale)}
                    lang={locale}
                    maxLength={120}
                  />
                  <AppTextarea
                    name={localeField("hint", locale)}
                    label={t(`${T}.fields.hint`)}
                    dir={localeDir(locale)}
                    lang={locale}
                    maxLength={500}
                    rows={2}
                  />
                </TabsContent>
              ))}
            </Tabs>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => change(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={pending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </AppForm>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
