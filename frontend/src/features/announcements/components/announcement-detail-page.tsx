"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Megaphone } from "lucide-react";
import { useEffect, type ReactNode } from "react";

import { EntityLink } from "@/components/entity";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { STATUS_TONE } from "@/features/announcements/components/announcements-manage-table";
import {
  announcementKeys,
  announcementsService,
  type AnnouncementAudience,
  type LocaleCode,
} from "@/features/announcements/services/announcements.service";
import { Markdown } from "@/features/portal/lib/markdown";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * Tenant > Announcements > detail (TEC-1218): one announcement in the
 * viewer's language with status, dates, audience, author and author
 * organization (linked) and the body rendered like the feed reader.
 */
export function AnnouncementDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canRead = can(Permission.AnnouncementsRead);
  const currentLocale = locale as LocaleCode;

  const detail = useQuery({
    queryKey: announcementKeys.detail(uuid, currentLocale),
    queryFn: () => announcementsService.get(uuid, currentLocale),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  // Opening the detail marks it read: refresh the feed and the badge.
  useEffect(() => {
    if (!detail.isSuccess) return;
    void qc.invalidateQueries({ queryKey: announcementKeys.lists() });
    void qc.invalidateQueries({
      queryKey: announcementKeys.unreadCount(currentLocale),
    });
  }, [currentLocale, qc, detail.isSuccess]);

  const a = detail.data;
  const listTitle = t("announcements.title");
  const title = a ? a.title : t("announcements.nav");
  const header = (
    <PageHeader
      title={title}
      icon={<Megaphone className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.announcements.list(slug) },
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
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("announcements.detail.not_found")
              : t("announcements.detail.error")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!a) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("announcements.detail.loading")}
        </p>
      </div>
    );
  }

  const when = (v: string | null | undefined) => (v ? format.dateTime(v) : "—");
  const audienceLabel = (x: AnnouncementAudience) => {
    const label = t(`announcements.audience.${x.target_type}`);
    return x.target_type === "role" && x.role_slug
      ? `${label} · ${x.role_slug}`
      : label;
  };
  const audiences = (a.audiences ?? []).map(audienceLabel);

  return (
    <div className="space-y-6" data-testid="announcement-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("announcements.detail.summary")}</CardTitle>
          <div className="flex items-center gap-2">
            {a.pinned ? (
              <StatusChip label={t("announcements.detail.pinned")} />
            ) : null}
            <StatusChip
              label={t(`announcements.status.${a.status}`)}
              tone={STATUS_TONE[a.status] ?? "default"}
            />
          </div>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("announcements.detail.author")}>
              <EntityLink kind="user" entity={a.author} slug={slug} />
            </Field>
            <Field label={t("announcements.detail.author_organization")}>
              <EntityLink
                kind="organization"
                entity={a.author_organization}
                slug={slug}
              />
            </Field>
            <Field label={t("announcements.form.fields.audience")}>
              {audiences.length ? audiences.join(", ") : "—"}
            </Field>
            <Field label={t("announcements.form.fields.publish_at")}>
              {when(a.publish_at)}
            </Field>
            <Field label={t("announcements.form.fields.expires_at")}>
              {when(a.expires_at)}
            </Field>
            <Field label={t("announcements.form.fields.pinned")}>
              {a.pinned ? t("table.true") : t("table.false")}
            </Field>
            <Field label={t("announcements.manage.created_at")}>
              {when(a.created_at)}
            </Field>
            <Field label={t("announcements.manage.updated_at")}>
              {when(a.updated_at)}
            </Field>
            <Field label={t("announcements.detail.read_at")}>
              {when(a.read_at)}
            </Field>
          </dl>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6" data-testid="announcement-body">
          <Markdown source={a.body} />
        </CardContent>
      </Card>
    </div>
  );
}
