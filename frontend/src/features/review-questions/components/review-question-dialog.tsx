"use client";

import { z } from "zod";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import {
  REVIEW_QUESTION_TARGETS,
  REVIEW_QUESTION_TEXT_MAX,
  REVIEW_QUESTION_TYPES,
  TARGET_LABEL_KEYS,
  TYPE_LABEL_KEYS,
  type ReviewQuestion,
  type ReviewQuestionInput,
  type ReviewQuestionTarget,
  type ReviewQuestionType,
} from "@/features/review-questions/services/review-questions.service";
import { useLocale } from "@/providers/locale-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Form field of a language tab ("zh-CN" → "text_zh_CN"). */
export const textField = (locale: AppLocale) =>
  `text_${locale.replace("-", "_")}`;

function questionSchema(t: Translate) {
  const texts = Object.fromEntries(
    SUPPORTED_LOCALES.map((locale) => [
      textField(locale),
      locale === "tr"
        ? z
            .string()
            .trim()
            .min(1, t("services.review_questions.validation.text_tr"))
            .max(REVIEW_QUESTION_TEXT_MAX)
        : z.string().trim().max(REVIEW_QUESTION_TEXT_MAX),
    ]),
  );
  return z.object({
    question_key: z
      .string()
      .trim()
      .min(1, t("services.review_questions.validation.key"))
      .max(64, t("services.review_questions.validation.key")),
    question_type: z.enum(["rating_1_5", "text"]),
    target: z.enum(["platform", "dealer", "product"]),
    is_required: z.boolean(),
    is_active: z.boolean(),
    ...texts,
  });
}

export type ReviewQuestionFormValues = {
  question_key: string;
  question_type: ReviewQuestionType;
  target: ReviewQuestionTarget;
  is_required: boolean;
  is_active: boolean;
} & Record<string, string | boolean>;

function defaults(question: ReviewQuestion | null): ReviewQuestionFormValues {
  const stored = new Map(
    (question?.locales ?? []).map((loc) => [loc.locale, loc.text]),
  );
  return {
    question_key: question?.question_key ?? "",
    question_type: question?.question_type ?? "rating_1_5",
    target: question?.target ?? "platform",
    is_required: question?.is_required ?? false,
    is_active: question?.is_active ?? true,
    ...Object.fromEntries(
      SUPPORTED_LOCALES.map((locale) => [
        textField(locale),
        stored.get(locale) ?? "",
      ]),
    ),
  };
}

/** Splits the form values into the question body and the per-language texts. */
export function splitValues(values: ReviewQuestionFormValues): {
  input: ReviewQuestionInput;
  texts: Record<string, string>;
} {
  return {
    input: {
      question_key: values.question_key.trim(),
      question_type: values.question_type,
      target: values.target,
      is_required: values.is_required,
      is_active: values.is_active,
    },
    texts: Object.fromEntries(
      SUPPORTED_LOCALES.map((locale) => [
        locale,
        String(values[textField(locale)] ?? ""),
      ]),
    ),
  };
}

type ReviewQuestionDialogProps = {
  open: boolean;
  question: ReviewQuestion | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: ReviewQuestionFormValues) => Promise<unknown>;
};

/**
 * Create / edit an admin review question: key, type, target, required,
 * active and the question text in 13 language tabs (Turkish required, the
 * portal falls back to it).
 */
export function ReviewQuestionDialog({
  open,
  question,
  pending,
  onOpenChange,
  onSubmit,
}: ReviewQuestionDialogProps) {
  const { t } = useLocale();
  const schema = questionSchema(t) as unknown as z.ZodType<
    ReviewQuestionFormValues,
    ReviewQuestionFormValues
  >;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {question
              ? t("services.review_questions.edit_title")
              : t("services.review_questions.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("services.review_questions.form_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <AppForm<ReviewQuestionFormValues>
            key={question?.uuid ?? "new"}
            schema={schema}
            defaultValues={defaults(question)}
            onSubmit={async (values) => {
              await onSubmit(values);
            }}
            className="space-y-4"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="question_key"
                label={t("services.review_questions.fields.key")}
                description={t("services.review_questions.key_hint")}
              />
              <AppSelect
                name="question_type"
                label={t("services.review_questions.fields.type")}
                options={REVIEW_QUESTION_TYPES.map((value) => ({
                  value,
                  label: t(TYPE_LABEL_KEYS[value]),
                }))}
              />
              <AppSelect
                name="target"
                label={t("services.review_questions.fields.target")}
                options={REVIEW_QUESTION_TARGETS.map((value) => ({
                  value,
                  label: t(TARGET_LABEL_KEYS[value]),
                }))}
              />
              <div className="flex flex-col justify-end gap-3">
                <AppSwitch
                  name="is_required"
                  label={t("services.review_questions.fields.required")}
                />
                <AppSwitch
                  name="is_active"
                  label={t("services.review_questions.fields.active")}
                />
              </div>
            </div>
            {question ? (
              <p className="text-muted-foreground text-xs">
                {t("services.review_questions.locked_hint")}
              </p>
            ) : null}
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
                  className="data-[state=inactive]:hidden"
                >
                  <AppTextarea
                    name={textField(locale)}
                    label={t("services.review_questions.fields.text")}
                    description={
                      locale === "tr"
                        ? t("services.review_questions.text_tr_hint")
                        : t("services.review_questions.text_hint")
                    }
                    dir={localeDir(locale)}
                    lang={locale}
                    maxLength={REVIEW_QUESTION_TEXT_MAX}
                    rows={3}
                  />
                </TabsContent>
              ))}
            </Tabs>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
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
