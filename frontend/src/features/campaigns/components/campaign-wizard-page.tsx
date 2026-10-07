"use client";

import { useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CAMPAIGN_CHANNELS,
  CHANNEL_LIMITS,
  audienceTypesFor,
  charCount,
  contentIssues,
  plansDirectly,
  requiredLocales,
  toZonedInput,
  type ContentIssue,
} from "@/features/campaigns/lib/campaigns";
import { useCampaignMutations } from "@/features/campaigns/hooks/use-campaigns";
import type {
  CampaignAudienceFilter,
  CampaignChannel,
  CampaignPreview,
  LocaleCode,
} from "@/features/campaigns/services/campaigns.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type ContentDraft = Record<LocaleCode, { title: string; body: string }>;

const DEFAULT_PREVIEW: CampaignPreview = {
  total: 0,
  locales: [{ locale: "tr", count: 0 }],
  channels: [],
  unreachable: 0,
  excluded: { total: 0, no_consent: 0, opted_out: 0 },
  missing_locales: ["tr"],
  sample: [],
};

function orgType(user: ReturnType<typeof useAuth>["user"]) {
  return user?.organizations?.[0]?.type;
}

function emptyAudience(type: CampaignAudienceFilter["audience_type"]) {
  return { audience_type: type } satisfies CampaignAudienceFilter;
}

function issueText(issue: ContentIssue) {
  const field = issue.field === "title" ? "title" : "body";
  return issue.kind === "required"
    ? `${issue.channel}.${field}.required`
    : `${issue.channel}.${field}.${issue.limit}`;
}

export function CampaignWizardPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { user } = useAuth();
  const router = useRouter();
  const mutations = useCampaignMutations();
  const type = orgType(user);
  const direct = plansDirectly(type);
  const audienceTypes = audienceTypesFor(type);
  const [name, setName] = useState("");
  const [channels, setChannels] = useState<CampaignChannel[]>(["push"]);
  const [audience, setAudience] = useState<CampaignAudienceFilter>(
    emptyAudience(audienceTypes[0] ?? "customers"),
  );
  const preview = DEFAULT_PREVIEW;
  const [contents, setContents] = useState<ContentDraft>({
    tr: { title: "", body: "" },
  } as ContentDraft);
  const [activeLocale, setActiveLocale] = useState<LocaleCode>("tr");
  const [scheduledAt, setScheduledAt] = useState("");

  const locales = requiredLocales(
    preview.locales,
    Object.keys(contents) as LocaleCode[],
  );
  const missingLocales = locales.filter(
    (locale) =>
      contentIssues(channels, contents[locale] ?? { title: "", body: "" })
        .length,
  );
  const currentIssues = contentIssues(
    channels,
    contents[activeLocale] ?? { title: "", body: "" },
  );
  const submitDisabled =
    !name.trim() || channels.length === 0 || missingLocales.length > 0;

  const previewChannels = useMemo(
    () =>
      CAMPAIGN_CHANNELS.map((channel) => ({
        channel,
        reachable:
          preview.channels.find((item) => item.channel === channel)
            ?.reachable ?? 0,
      })),
    [preview.channels],
  );

  const toggleChannel = (channel: CampaignChannel, checked: boolean) => {
    setChannels((current) =>
      checked
        ? [...new Set([...current, channel])]
        : current.filter((c) => c !== channel),
    );
  };

  const updateContent = (
    locale: LocaleCode,
    field: "title" | "body",
    value: string,
  ) => {
    setContents((current) => ({
      ...current,
      [locale]: {
        title: current[locale]?.title ?? "",
        body: current[locale]?.body ?? "",
        [field]: value,
      },
    }));
  };

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (submitDisabled) return;
    try {
      const campaign = await mutations.create.mutateAsync({
        name: name.trim(),
        channels,
        audience_filter: audience,
      });
      for (const locale of locales) {
        const content = contents[locale];
        if (!content) continue;
        await mutations.putContent.mutateAsync({
          uuid: campaign.uuid,
          locale,
          body: { ...content, deeplink: null },
        });
      }
      if (direct && scheduledAt) {
        await mutations.schedule.mutateAsync({
          uuid: campaign.uuid,
          scheduledAt,
        });
      } else {
        await mutations.submit.mutateAsync(campaign.uuid);
      }
      toast.success(t("campaigns.toast.submitted"));
      router.push(routes.tenant.campaigns.detail(slug, campaign.uuid));
    } catch {
      toast.error(t("campaigns.toast.failed"));
    }
  };

  return (
    <EntityPage
      title={t("campaigns.wizard.title")}
      description={t("campaigns.wizard.description")}
      permission={permissions.campaigns.write}
      forbiddenFallback={<ErrorState title={t("common.error_forbidden")} />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("campaigns.title"),
          href: routes.tenant.campaigns.list(slug),
        },
        { label: t("campaigns.wizard.title") },
      ]}
    >
      <form className="space-y-4" onSubmit={(event) => void onSubmit(event)}>
        <Card>
          <CardHeader>
            <CardTitle>{t("campaigns.wizard.step_basics")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="campaign-name">
                {t("campaigns.fields.name")}
              </Label>
              <Input
                id="campaign-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">
                {t("campaigns.fields.channels")}
              </legend>
              <div className="flex flex-wrap gap-3">
                {CAMPAIGN_CHANNELS.map((channel) => (
                  <label
                    key={channel}
                    className="flex items-center gap-2 text-sm"
                  >
                    <Checkbox
                      data-testid={`campaign-channel-${channel}`}
                      checked={channels.includes(channel)}
                      onCheckedChange={(v) =>
                        toggleChannel(channel, v === true)
                      }
                    />
                    {t(`campaigns.channel.${channel}`)}
                  </label>
                ))}
              </div>
            </fieldset>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("campaigns.wizard.step_audience")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div className="space-y-2">
              <Label htmlFor="audience-type">
                {t("campaigns.fields.audience_type")}
              </Label>
              <select
                id="audience-type"
                className="border-input bg-background h-9 rounded-md border px-3 text-sm"
                value={audience.audience_type}
                onChange={(event) =>
                  setAudience(
                    emptyAudience(
                      event.target
                        .value as CampaignAudienceFilter["audience_type"],
                    ),
                  )
                }
              >
                {audienceTypes.map((value) => (
                  <option key={value} value={value}>
                    {t(`campaigns.audience.${value}`)}
                  </option>
                ))}
              </select>
            </div>
            <div className="bg-muted/40 rounded-md p-3 text-sm">
              <div data-testid="campaign-preview-total">
                {t("campaigns.preview.total", { count: preview.total })}
              </div>
              <div>
                {t("campaigns.preview.languages")}:{" "}
                {preview.locales
                  .map((l) => `${l.locale} ${l.count}`)
                  .join(", ")}
              </div>
              <div>
                {t("campaigns.preview.reach")}:{" "}
                {previewChannels
                  .map(
                    (c) =>
                      `${t(`campaigns.channel.${c.channel}`)} ${c.reachable}`,
                  )
                  .join(", ")}
              </div>
              <div>
                {t("campaigns.preview.excluded", {
                  count: preview.excluded.total,
                })}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("campaigns.wizard.step_content")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Tabs
              value={activeLocale}
              onValueChange={(v) => setActiveLocale(v as LocaleCode)}
            >
              <TabsList className="flex-wrap">
                {locales.map((locale) => (
                  <TabsTrigger
                    key={locale}
                    value={locale}
                    data-testid={`campaign-locale-tab-${locale}`}
                    className={
                      missingLocales.includes(locale)
                        ? "text-destructive"
                        : undefined
                    }
                  >
                    {locale}
                  </TabsTrigger>
                ))}
              </TabsList>
              {locales.map((locale) => {
                const content = contents[locale] ?? { title: "", body: "" };
                return (
                  <TabsContent
                    key={locale}
                    value={locale}
                    className="space-y-3"
                  >
                    <div className="space-y-2">
                      <Label htmlFor={`campaign-title-${locale}`}>
                        {t("campaigns.fields.title")}
                      </Label>
                      <Input
                        id={`campaign-title-${locale}`}
                        value={content.title}
                        onChange={(event) =>
                          updateContent(locale, "title", event.target.value)
                        }
                        aria-invalid={
                          content.title.length > CHANNEL_LIMITS.push.title
                        }
                      />
                      {channels.includes("push") ? (
                        <p
                          className="text-muted-foreground text-xs"
                          data-testid="campaign-push-title-count"
                        >
                          {charCount(content.title)}/{CHANNEL_LIMITS.push.title}
                        </p>
                      ) : null}
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`campaign-body-${locale}`}>
                        {t("campaigns.fields.body")}
                      </Label>
                      <Textarea
                        id={`campaign-body-${locale}`}
                        value={content.body}
                        onChange={(event) =>
                          updateContent(locale, "body", event.target.value)
                        }
                        rows={5}
                      />
                    </div>
                    <Input
                      type="file"
                      accept="image/jpeg,image/png,image/webp,application/pdf"
                      aria-label={t("campaigns.fields.media")}
                    />
                    <div
                      dir={locale === "ar" ? "rtl" : "ltr"}
                      className="border-border rounded-md border p-3"
                      data-testid="campaign-preview-card"
                    >
                      <div className="font-medium">
                        {content.title || t("campaigns.preview_card.title")}
                      </div>
                      <p className="text-muted-foreground text-sm">
                        {content.body || t("campaigns.preview_card.body")}
                      </p>
                    </div>
                  </TabsContent>
                );
              })}
            </Tabs>
            {missingLocales.length ? (
              <div
                className="text-destructive text-sm"
                data-testid="campaign-missing-locales"
              >
                {t("campaigns.validation.missing_languages")}:{" "}
                {missingLocales.join(", ")}
              </div>
            ) : null}
            {currentIssues.length ? (
              <ul className="text-destructive text-sm">
                {currentIssues.map((issue) => (
                  <li key={issueText(issue)}>
                    {t(`campaigns.validation.${issue.kind}`, {
                      channel: t(`campaigns.channel.${issue.channel}`),
                      field: t(`campaigns.fields.${issue.field}`),
                      limit: issue.limit ?? "",
                    })}
                  </li>
                ))}
              </ul>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("campaigns.wizard.step_schedule")}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-end gap-3">
            <div className="space-y-2">
              <Label htmlFor="campaign-scheduled-at">
                {t("campaigns.fields.scheduled_at")}
              </Label>
              <Input
                id="campaign-scheduled-at"
                type="datetime-local"
                value={scheduledAt}
                placeholder={toZonedInput(
                  new Date().toISOString(),
                  user?.timeZone ?? "Europe/Istanbul",
                )}
                onChange={(event) => setScheduledAt(event.target.value)}
              />
            </div>
            <Button
              type="submit"
              disabled={submitDisabled || mutations.create.isPending}
              data-testid="campaign-submit"
            >
              {direct
                ? t("campaigns.actions.schedule")
                : t("campaigns.actions.submit_approval")}
            </Button>
          </CardContent>
        </Card>
      </form>
    </EntityPage>
  );
}
