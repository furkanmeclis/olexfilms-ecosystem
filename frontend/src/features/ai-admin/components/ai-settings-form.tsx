"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { QuotaInput } from "@/features/ai-admin/components/quota-input";
import {
  AI_INSTRUCTIONS_MAX_CHARS,
  AI_KNOWLEDGE_MAX_BYTES,
  aiUserName,
  byteLength,
} from "@/features/ai-admin/lib/quota";
import {
  aiSettingsSchema,
  toAISettingsFormValues,
  toAISettingsUpdate,
  type AISettingsFormValues,
} from "@/features/ai-admin/lib/settings-form";
import type {
  AISettings,
  AISettingsUpdate,
  AIToolInfo,
} from "@/features/ai-admin/services/ai-admin.service";
import { Markdown } from "@/features/portal/lib/markdown";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const REALMS: AIToolInfo["realm"][] = ["panel", "customer", "visitor"];

type AISettingsFormProps = {
  settings: AISettings;
  isSaving?: boolean;
  onSubmit: (body: AISettingsUpdate) => Promise<unknown> | void;
};

/**
 * Platform AI settings (TEC-391): models from the allow list, default and
 * system pool quotas (0 = unlimited), tool switches, extra instructions and
 * the Markdown knowledge text. Remount with a new `key` to reset.
 */
export function AISettingsForm({
  settings,
  isSaving,
  onSubmit,
}: AISettingsFormProps) {
  const { t, format } = useLocale();
  const form = useForm<AISettingsFormValues>({
    resolver: zodResolver(aiSettingsSchema as never),
    defaultValues: toAISettingsFormValues(settings),
  });
  const { control, formState, register } = form;
  const errors = formState.errors;
  const err = (message: string | undefined) =>
    message ? (
      <p className="text-destructive text-sm" role="alert">
        {t(message)}
      </p>
    ) : null;

  const models = useMemo(() => {
    const set = new Set(settings.allowed_models);
    set.add(settings.default_model);
    set.add(settings.fast_model);
    return [...set].filter(Boolean);
  }, [settings]);

  const toolsByRealm = useMemo(
    () =>
      REALMS.map((realm) => ({
        realm,
        tools: settings.tools
          .filter((tool) => tool.realm === realm)
          .sort((a, b) => a.name.localeCompare(b.name)),
      })).filter((group) => group.tools.length > 0),
    [settings.tools],
  );

  const instructions = useWatch({ control, name: "extra_instructions" }) ?? "";
  const knowledge = useWatch({ control, name: "knowledge_text" }) ?? "";
  const knowledgeBytes = byteLength(knowledge);

  const submit = form.handleSubmit(async (values) => {
    await onSubmit(toAISettingsUpdate(values));
    form.reset(values);
  });

  return (
    <form className="space-y-6" onSubmit={submit} noValidate>
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("ai_admin.settings.models_title")}</CardTitle>
            <CardDescription>
              {t("ai_admin.settings.models_description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {(["default_model", "fast_model"] as const).map((name) => (
              <div key={name} className="space-y-2">
                <Label htmlFor={`ai-${name}`}>
                  {t(`ai_admin.settings.${name}`)}
                </Label>
                <Controller
                  control={control}
                  name={name}
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id={`ai-${name}`} dir="ltr">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {models.map((model) => (
                          <SelectItem key={model} value={model}>
                            {model}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
                <p className="text-muted-foreground text-xs">
                  {t(`ai_admin.settings.${name}_hint`)}
                </p>
                {err(errors[name]?.message)}
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("ai_admin.settings.quotas_title")}</CardTitle>
            <CardDescription>
              {t("ai_admin.settings.quotas_description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {(
              [
                "default_monthly_token_quota",
                "system_pool_monthly_quota",
              ] as const
            ).map((name) => (
              <div key={name} className="space-y-2">
                <Label htmlFor={`ai-${name}`}>
                  {t(`ai_admin.settings.${name}`)}
                </Label>
                <Controller
                  control={control}
                  name={name}
                  render={({ field }) => (
                    <QuotaInput
                      id={`ai-${name}`}
                      value={field.value}
                      onChange={field.onChange}
                      invalid={Boolean(errors[name])}
                    />
                  )}
                />
                <p className="text-muted-foreground text-xs">
                  {t(`ai_admin.settings.${name}_hint`)}
                </p>
                {err(errors[name]?.message)}
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("ai_admin.settings.tools_title")}</CardTitle>
          <CardDescription>
            {t("ai_admin.settings.tools_description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {toolsByRealm.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("ai_admin.settings.tools_empty")}
            </p>
          ) : null}
          {toolsByRealm.map((group) => (
            <section key={group.realm} className="space-y-2">
              <h3 className="text-sm font-medium">
                {t(`ai_admin.realm.${group.realm}`)}
              </h3>
              <ul className="divide-y rounded-md border">
                {group.tools.map((tool) => (
                  <li
                    key={tool.name}
                    className="flex items-center justify-between gap-4 px-3 py-2"
                  >
                    <label
                      htmlFor={`ai-tool-${tool.name}`}
                      className="flex min-w-0 items-center gap-2"
                    >
                      <code className="truncate text-sm" dir="ltr">
                        {tool.name}
                      </code>
                      <Badge
                        variant={tool.kind === "write" ? "warning" : "outline"}
                        className="font-normal"
                      >
                        {t(`ai_admin.tool_kind.${tool.kind}`)}
                      </Badge>
                    </label>
                    <Controller
                      control={control}
                      name={`tools.${tool.name}`}
                      render={({ field }) => (
                        <Switch
                          id={`ai-tool-${tool.name}`}
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      )}
                    />
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ai_admin.settings.instructions_title")}</CardTitle>
          <CardDescription>
            {t("ai_admin.settings.instructions_description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          <Textarea
            id="ai-extra-instructions"
            rows={6}
            aria-label={t("ai_admin.settings.instructions_title")}
            {...register("extra_instructions")}
          />
          <p
            className={cn(
              "text-muted-foreground text-end text-xs tabular-nums",
              instructions.length > AI_INSTRUCTIONS_MAX_CHARS &&
                "text-destructive",
            )}
          >
            {format.number(instructions.length)} /{" "}
            {format.number(AI_INSTRUCTIONS_MAX_CHARS)}
          </p>
          {err(errors.extra_instructions?.message)}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ai_admin.settings.knowledge_title")}</CardTitle>
          <CardDescription>
            {t("ai_admin.settings.knowledge_description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          <Tabs defaultValue="write">
            <TabsList>
              <TabsTrigger value="write">
                {t("ai_admin.settings.knowledge_write")}
              </TabsTrigger>
              <TabsTrigger value="preview">
                {t("ai_admin.settings.knowledge_preview")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="write">
              <Textarea
                id="ai-knowledge-text"
                rows={16}
                className="font-mono text-sm"
                aria-label={t("ai_admin.settings.knowledge_title")}
                {...register("knowledge_text")}
              />
            </TabsContent>
            <TabsContent value="preview">
              <div className="min-h-40 rounded-md border p-4">
                {knowledge.trim() ? (
                  <Markdown source={knowledge} />
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("ai_admin.settings.knowledge_empty")}
                  </p>
                )}
              </div>
            </TabsContent>
          </Tabs>
          <p
            className={cn(
              "text-muted-foreground text-end text-xs tabular-nums",
              knowledgeBytes > AI_KNOWLEDGE_MAX_BYTES && "text-destructive",
            )}
          >
            {t("ai_admin.settings.knowledge_size", {
              used: format.number(Math.ceil(knowledgeBytes / 102.4) / 10),
              max: format.number(AI_KNOWLEDGE_MAX_BYTES / 1024),
            })}
          </p>
          {err(errors.knowledge_text?.message)}
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-xs">
          {settings.updated_by
            ? t("ai_admin.settings.updated_by", {
                name: aiUserName(settings.updated_by),
                time: format.dateTime(settings.updated_at),
              })
            : t("ai_admin.settings.updated_at", {
                time: format.dateTime(settings.updated_at),
              })}
        </p>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={!formState.isDirty || isSaving}
            onClick={() => form.reset()}
          >
            {t("ai_admin.actions.reset")}
          </Button>
          <Button type="submit" disabled={!formState.isDirty || isSaving}>
            {t("ai_admin.actions.save")}
          </Button>
        </div>
      </div>
    </form>
  );
}
