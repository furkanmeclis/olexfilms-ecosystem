"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Bell,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Eye,
  Megaphone,
  Pin,
  Plus,
  Send,
} from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ANNOUNCEMENT_LOCALES,
  ANNOUNCEMENT_PAGE_SIZE,
  audienceOptions,
  buildAudience,
  defaultAnnouncementForm,
  isUnread,
  readRatePercent,
  sortedAnnouncements,
  toIsoOrNull,
  validateAnnouncementForm,
  type AnnouncementFormValues,
} from "@/features/announcements/lib/announcements";
import {
  announcementKeys,
  announcementsService,
  type Announcement,
  type LocaleCode,
} from "@/features/announcements/services/announcements.service";
import { Markdown } from "@/features/portal/lib/markdown";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const inputClass =
  "border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const selectClass = inputClass;

function pageCount(total: number) {
  return Math.max(1, Math.ceil(total / ANNOUNCEMENT_PAGE_SIZE));
}

function localeLabel(locale: LocaleCode): string {
  return locale === "zh-CN" ? "ZH" : locale.toUpperCase();
}

function announcementDate(a: Announcement): string | undefined {
  return a.publish_at ?? a.created_at;
}

function AnnouncementList({
  items,
  selectedUuid,
  onSelect,
}: {
  items: Announcement[];
  selectedUuid: string | null;
  onSelect: (uuid: string) => void;
}) {
  const { t, format } = useLocale();
  if (!items.length) {
    return (
      <p className="text-muted-foreground p-6 text-sm">
        {t("announcements.list.empty")}
      </p>
    );
  }
  return (
    <ul className="divide-y" data-testid="announcement-list">
      {sortedAnnouncements(items).map((item) => {
        const unread = isUnread(item);
        return (
          <li key={item.uuid}>
            <button
              type="button"
              data-testid="announcement-row"
              data-uuid={item.uuid}
              data-unread={unread}
              className={cn(
                "hover:bg-muted/50 flex w-full items-start gap-3 px-4 py-3 text-start transition-colors",
                selectedUuid === item.uuid && "bg-muted",
                unread && "border-s-primary bg-primary/5 border-s-4",
              )}
              onClick={() => onSelect(item.uuid)}
            >
              <span
                className={cn(
                  "mt-1 size-2 rounded-full",
                  unread ? "bg-primary" : "bg-muted-foreground/30",
                )}
                aria-hidden
              />
              <span className="min-w-0 flex-1 space-y-1">
                <span className="flex items-center gap-2">
                  {item.pinned ? <Pin className="size-3.5" /> : null}
                  <span className="truncate text-sm font-medium">
                    {item.title}
                  </span>
                </span>
                <span className="text-muted-foreground line-clamp-2 text-xs">
                  {item.body}
                </span>
                {announcementDate(item) ? (
                  <span className="text-muted-foreground text-xs">
                    {format.dateTime(announcementDate(item)!)}
                  </span>
                ) : null}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function ReadReport({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const query = useQuery({
    queryKey: announcementKeys.reads(uuid),
    queryFn: () => announcementsService.reads(uuid),
  });

  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  const report = query.data;
  const rate = report ? readRatePercent(report.read_rate) : 0;

  return (
    <Card data-testid="announcement-read-report">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Eye className="size-4" />
          {t("announcements.report.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div>
          <div className="mb-1 flex justify-between text-sm">
            <span>{t("announcements.report.rate")}</span>
            <span className="font-medium">{rate}%</span>
          </div>
          <div className="bg-muted h-2 overflow-hidden rounded-full">
            <div
              className="bg-primary h-full"
              style={{ inlineSize: `${Math.min(100, Math.max(0, rate))}%` }}
            />
          </div>
          <p className="text-muted-foreground mt-2 text-xs">
            {t("announcements.report.counts", {
              read: report?.read_total ?? 0,
              total: report?.target_total ?? 0,
            })}
          </p>
        </div>
        <ul className="divide-y rounded-md border">
          {(report?.items ?? []).map((item) => (
            <li key={item.user_uuid} className="px-3 py-2 text-sm">
              <div className="font-medium">
                {[item.name, item.surname].filter(Boolean).join(" ") ||
                  item.email}
              </div>
              <div className="text-muted-foreground text-xs">
                {item.email} · {format.dateTime(item.read_at)}
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

function AnnouncementDetail({
  uuid,
  locale,
  canWrite,
}: {
  uuid: string | null;
  locale: LocaleCode;
  canWrite: boolean;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: uuid ? announcementKeys.detail(uuid, locale) : ["announcements"],
    queryFn: () => announcementsService.get(uuid!, locale),
    enabled: Boolean(uuid),
  });

  useEffect(() => {
    if (!query.isSuccess) return;
    void qc.invalidateQueries({ queryKey: announcementKeys.lists() });
    void qc.invalidateQueries({
      queryKey: announcementKeys.unreadCount(locale),
    });
  }, [locale, qc, query.isSuccess]);

  if (!uuid) {
    return (
      <Card>
        <CardContent className="text-muted-foreground flex min-h-64 items-center justify-center text-sm">
          {t("announcements.detail.pick")}
        </CardContent>
      </Card>
    );
  }

  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={
          isApiError(query.error)
            ? query.error.message
            : t("announcements.detail.error")
        }
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  const item = query.data;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {item?.read_at ? (
              <CheckCircle2 className="size-4 text-emerald-600" />
            ) : (
              <Bell className="text-primary size-4" />
            )}
            {item?.title ?? t("announcements.detail.loading")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {item ? (
            <>
              <div className="text-muted-foreground flex flex-wrap gap-3 text-xs">
                <span>{t(`announcements.status.${item.status}`)}</span>
                {item.pinned ? (
                  <span>{t("announcements.detail.pinned")}</span>
                ) : null}
                {announcementDate(item) ? (
                  <span>{format.dateTime(announcementDate(item)!)}</span>
                ) : null}
              </div>
              <Markdown source={item.body} />
            </>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("announcements.detail.loading")}
            </p>
          )}
        </CardContent>
      </Card>
      {canWrite && item ? <ReadReport uuid={item.uuid} /> : null}
    </div>
  );
}

function AnnouncementComposer({
  slug,
  onCreated,
}: {
  slug: string;
  onCreated: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const org = useActiveOrganization(slug);
  const qc = useQueryClient();
  const options = audienceOptions(org?.type);
  const [activeLocale, setActiveLocale] = useState<LocaleCode>(
    (locale as LocaleCode) || "tr",
  );
  const [values, setValues] = useState<AnnouncementFormValues>(() =>
    defaultAnnouncementForm(locale, org?.type),
  );
  const [errors, setErrors] = useState<Record<string, string>>({});
  const current = values.locales[activeLocale];

  const mutation = useMutation({
    mutationFn: async (draft: AnnouncementFormValues) => {
      const defaultDraft = draft.locales[draft.defaultLocale];
      const created = await announcementsService.create({
        default_locale: draft.defaultLocale,
        title: defaultDraft.title.trim(),
        body: defaultDraft.body.trim(),
        body_format: "markdown",
        pinned: draft.pinned,
        notify: draft.notify,
        publish_at: toIsoOrNull(draft.publishAt),
        expires_at: toIsoOrNull(draft.expiresAt),
        audiences: buildAudience(draft),
      });
      for (const localeCode of ANNOUNCEMENT_LOCALES) {
        if (localeCode === draft.defaultLocale) continue;
        const localized = draft.locales[localeCode];
        if (!localized.title.trim() && !localized.body.trim()) continue;
        await announcementsService.putLocale(created.uuid, localeCode, {
          title: localized.title.trim(),
          body: localized.body.trim(),
        });
      }
      return announcementsService.publish(created.uuid);
    },
    onSuccess: (created) => {
      toast.success(t("announcements.form.created"));
      setValues(defaultAnnouncementForm(locale, org?.type));
      setActiveLocale((locale as LocaleCode) || "tr");
      void qc.invalidateQueries({ queryKey: announcementKeys.lists() });
      void qc.invalidateQueries({
        queryKey: announcementKeys.unreadCount(locale as LocaleCode),
      });
      onCreated(created.uuid);
    },
    onError: (err) => {
      toast.error(
        isApiError(err) ? err.message : t("announcements.form.create_error"),
      );
    },
  });

  const patchLocale = (patch: Partial<{ title: string; body: string }>) => {
    setValues((prev) => ({
      ...prev,
      locales: {
        ...prev.locales,
        [activeLocale]: { ...prev.locales[activeLocale], ...patch },
      },
    }));
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const localErrors = validateAnnouncementForm(values);
    setErrors(
      Object.fromEntries(
        Object.entries(localErrors).map(([key, value]) => [key, t(value)]),
      ),
    );
    if (Object.keys(localErrors).length) return;
    mutation.mutate(values);
  };

  return (
    <form onSubmit={onSubmit} noValidate data-testid="announcement-form">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Plus className="size-4" />
            {t("announcements.form.title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex flex-wrap gap-1" role="tablist">
            {ANNOUNCEMENT_LOCALES.map((localeCode) => (
              <button
                key={localeCode}
                type="button"
                role="tab"
                aria-selected={activeLocale === localeCode}
                data-testid="announcement-locale-tab"
                className={cn(
                  "rounded-md border px-2 py-1 text-xs font-medium",
                  activeLocale === localeCode
                    ? "bg-primary text-primary-foreground"
                    : "bg-background hover:bg-muted",
                )}
                onClick={() => setActiveLocale(localeCode)}
              >
                {localeLabel(localeCode)}
              </button>
            ))}
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="announcement-title">
                  {t("announcements.form.fields.title")}
                </Label>
                <input
                  id="announcement-title"
                  data-testid="announcement-title"
                  className={inputClass}
                  value={current.title}
                  onChange={(e) => patchLocale({ title: e.target.value })}
                />
                {errors.title ? (
                  <p className="text-destructive text-xs">{errors.title}</p>
                ) : null}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="announcement-body">
                  {t("announcements.form.fields.body")}
                </Label>
                <Textarea
                  id="announcement-body"
                  data-testid="announcement-body"
                  rows={10}
                  value={current.body}
                  onChange={(e) => patchLocale({ body: e.target.value })}
                />
                {errors.body ? (
                  <p className="text-destructive text-xs">{errors.body}</p>
                ) : null}
              </div>
            </div>
            <div className="space-y-2 rounded-md border p-3">
              <div className="text-sm font-medium">
                {t("announcements.form.preview")}
              </div>
              <div data-testid="announcement-preview">
                <Markdown source={current.body} />
              </div>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="announcement-default-locale">
                {t("announcements.form.fields.default_locale")}
              </Label>
              <select
                id="announcement-default-locale"
                className={selectClass}
                value={values.defaultLocale}
                onChange={(e) =>
                  setValues((v) => ({
                    ...v,
                    defaultLocale: e.target.value as LocaleCode,
                  }))
                }
              >
                {ANNOUNCEMENT_LOCALES.map((localeCode) => (
                  <option key={localeCode} value={localeCode}>
                    {localeLabel(localeCode)}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="announcement-audience">
                {t("announcements.form.fields.audience")}
              </Label>
              <select
                id="announcement-audience"
                data-testid="announcement-audience"
                className={selectClass}
                value={values.audience}
                onChange={(e) =>
                  setValues((v) => ({
                    ...v,
                    audience: e.target
                      .value as AnnouncementFormValues["audience"],
                  }))
                }
              >
                {options.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </option>
                ))}
              </select>
            </div>
            {values.audience === "role" ? (
              <div className="space-y-1.5">
                <Label htmlFor="announcement-role">
                  {t("announcements.form.fields.role")}
                </Label>
                <input
                  id="announcement-role"
                  className={inputClass}
                  value={values.roleSlug}
                  onChange={(e) =>
                    setValues((v) => ({ ...v, roleSlug: e.target.value }))
                  }
                />
                {errors.role ? (
                  <p className="text-destructive text-xs">{errors.role}</p>
                ) : null}
              </div>
            ) : null}
            <div className="space-y-1.5">
              <Label htmlFor="announcement-publish-at">
                {t("announcements.form.fields.publish_at")}
              </Label>
              <input
                id="announcement-publish-at"
                data-testid="announcement-publish-at"
                type="datetime-local"
                className={inputClass}
                value={values.publishAt}
                onChange={(e) =>
                  setValues((v) => ({ ...v, publishAt: e.target.value }))
                }
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="announcement-expires-at">
                {t("announcements.form.fields.expires_at")}
              </Label>
              <input
                id="announcement-expires-at"
                data-testid="announcement-expires-at"
                type="datetime-local"
                className={inputClass}
                value={values.expiresAt}
                onChange={(e) =>
                  setValues((v) => ({ ...v, expiresAt: e.target.value }))
                }
              />
              {errors.expiresAt ? (
                <p
                  className="text-destructive text-xs"
                  data-testid="announcement-date-error"
                >
                  {errors.expiresAt}
                </p>
              ) : null}
            </div>
          </div>

          <div className="flex flex-wrap gap-4">
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={values.pinned}
                onCheckedChange={(checked) =>
                  setValues((v) => ({ ...v, pinned: checked === true }))
                }
              />
              {t("announcements.form.fields.pinned")}
            </label>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={values.notify}
                onCheckedChange={(checked) =>
                  setValues((v) => ({ ...v, notify: checked === true }))
                }
              />
              {t("announcements.form.fields.notify")}
            </label>
          </div>

          <div className="flex justify-end">
            <Button type="submit" disabled={mutation.isPending}>
              <Send className="size-4" />
              {t("announcements.form.publish")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}

export function AnnouncementsPage({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.AnnouncementsRead);
  const canWrite = can(Permission.AnnouncementsWrite);
  const [page, setPage] = useState(0);
  const [selectedUuid, setSelectedUuid] = useState<string | null>(null);
  const currentLocale = locale as LocaleCode;
  const query = useMemo(
    () => ({
      locale: currentLocale,
      limit: ANNOUNCEMENT_PAGE_SIZE,
      offset: page * ANNOUNCEMENT_PAGE_SIZE,
    }),
    [currentLocale, page],
  );
  const list = useQuery({
    queryKey: announcementKeys.list(query),
    queryFn: () => announcementsService.list(query),
    enabled: canRead,
    placeholderData: keepPreviousData,
  });

  const title = t("announcements.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Megaphone className="size-6" />}
      description={t("announcements.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("announcements.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = pageCount(total);
  const rows = list.data?.items ?? [];

  return (
    <div className="space-y-6">
      {header}
      <div className="grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("announcements.list.title")}</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {list.isError ? (
                <div className="p-6">
                  <ErrorState
                    title={t("common.error_generic")}
                    onRetry={() => void list.refetch()}
                    retryLabel={t("common.retry")}
                  />
                </div>
              ) : (
                <AnnouncementList
                  items={rows}
                  selectedUuid={selectedUuid}
                  onSelect={setSelectedUuid}
                />
              )}
            </CardContent>
          </Card>
          <div className="flex items-center justify-between">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page <= 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
            >
              <ChevronLeft className="size-4" />
              {t("announcements.list.previous")}
            </Button>
            <span className="text-muted-foreground text-xs">
              {t("announcements.list.page", {
                page: page + 1,
                total: pages,
              })}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page + 1 >= pages}
              onClick={() => setPage((p) => p + 1)}
            >
              {t("announcements.list.next")}
              <ChevronRight className="size-4" />
            </Button>
          </div>
          {canWrite ? (
            <AnnouncementComposer slug={slug} onCreated={setSelectedUuid} />
          ) : null}
        </div>
        <AnnouncementDetail
          uuid={selectedUuid}
          locale={currentLocale}
          canWrite={canWrite}
        />
      </div>
    </div>
  );
}
