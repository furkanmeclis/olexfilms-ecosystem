"use client";

import { useQuery } from "@tanstack/react-query";
import { UserRound } from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityLink, RelatedList } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { staffReportsService } from "@/features/staff-reports/services/staff-reports.service";
import { UserActivitySection } from "@/features/users/components/user-activity-section";
import type { components } from "@/generated/api";
import { isApiError, platformRequest } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type MemberSummary = components["schemas"]["MemberSummary"];

/** GET /v1/tenant/users/{uuid}/summary (TEC-1216, members.read scope). */
export function getMemberSummary(uuid: string) {
  return platformRequest<MemberSummary>(
    "GET",
    `/v1/tenant/users/${encodeURIComponent(uuid)}/summary`,
  );
}

export const memberKeys = {
  summary: (uuid: string) => ["members", "summary", uuid] as const,
  staffProfile: (uuid: string) => ["members", "staff-profile", uuid] as const,
};

export type MemberTab =
  | "services_created"
  | "services_performed"
  | "orders"
  | "tasks"
  | "leads"
  | "payroll"
  | "activity";

/**
 * Tabs the viewer may open: each needs the module's read permission;
 * payroll needs staff.manage (personal data, TEC-349), activity the
 * activity log permission (TEC-1218).
 */
export function memberTabs(can: (permission: string) => boolean): MemberTab[] {
  const tabs: MemberTab[] = [];
  if (can(Permission.ServicesRead)) {
    tabs.push("services_created", "services_performed");
  }
  if (can(Permission.OrdersRead)) tabs.push("orders");
  if (can(Permission.TasksRead)) tabs.push("tasks");
  if (can(Permission.LeadsRead)) tabs.push("leads");
  if (can(Permission.StaffManage)) tabs.push("payroll");
  if (can(Permission.PlatformActivityRead)) tabs.push("activity");
  return tabs;
}

/** Display name of a system role slug (falls back to the slug). */
function roleLabel(slug: string, t: (key: string) => string) {
  const key = `roles.names.${slug}`;
  const label = t(key);
  return label === key ? slug : label;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * The staff card (payroll) of the user, when one exists: links the
 * existing staff detail page (TEC-349) instead of repeating it.
 */
function PayrollTab({ slug, userUuid }: { slug: string; userUuid: string }) {
  const { t } = useLocale();
  const profile = useQuery({
    queryKey: memberKeys.staffProfile(userUuid),
    queryFn: async () => {
      const items = await staffReportsService.listStaff();
      return items.items.find((s) => s.user_uuid === userUuid) ?? null;
    },
  });
  if (profile.isLoading) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
    );
  }
  if (profile.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void profile.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (!profile.data) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="member-payroll">
        {t("users.member.payroll_none")}
      </p>
    );
  }
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      data-testid="member-payroll"
    >
      <p className="text-sm">{t("users.member.payroll_hint")}</p>
      <Button asChild variant="outline" size="sm">
        <Link href={routes.tenant.staff.detail(slug, profile.data.uuid)}>
          {t("users.member.payroll_open")}
        </Link>
      </Button>
    </div>
  );
}

/**
 * Tenant > user (member) detail (TEC-1218): identity, memberships with
 * roles and warehouses, counters and what the user did — services they
 * opened or performed, orders they opened, tasks assigned, leads assigned,
 * their staff card and the activity log. Every user ref links here.
 */
export function MemberDetailPage({
  slug,
  uuid,
  initialTab,
}: {
  slug: string;
  uuid: string;
  initialTab?: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.MembersRead);

  const summary = useQuery({
    queryKey: memberKeys.summary(uuid),
    queryFn: () => getMemberSummary(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  const tabs = useMemo(() => memberTabs(can), [can]);
  const [tab, setTab] = useState<string | undefined>(initialTab);
  const active =
    tab && tabs.includes(tab as MemberTab) ? (tab as MemberTab) : tabs[0];

  const filters = useMemo(
    () => ({
      services_created: { created_by_uuid: uuid },
      services_performed: { performer_uuid: uuid },
      orders: { created_by_uuid: uuid },
      tasks: { assignee_uuid: uuid },
      leads: { assignee_uuid: uuid },
    }),
    [uuid],
  );

  const m = summary.data;
  const name = m
    ? [m.user.name, m.user.surname].filter(Boolean).join(" ") || m.ref.name
    : t("entity.kinds.user");
  const header = (
    <PageHeader
      title={name}
      icon={<UserRound className="size-6" />}
      description={m?.user.email ?? undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: name },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("users.member.forbidden")}
        />
      </div>
    );
  }
  if (summary.isError) {
    const notFound = isApiError(summary.error) && summary.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound ? t("users.member.not_found") : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void summary.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!m) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      </div>
    );
  }

  const count = (value: number | null) =>
    value === null ? null : format.number(value);
  const counters: { key: string; value: number | null }[] = [
    { key: "services_created", value: m.counts.services_created },
    { key: "services_performed", value: m.counts.services_performed },
    { key: "orders_created", value: m.counts.orders_created },
    { key: "open_tasks", value: m.counts.open_tasks },
    { key: "leads_assigned", value: m.counts.leads_assigned },
  ].filter((c) => c.value !== null);

  return (
    <div className="space-y-6" data-testid="member-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("users.member.summary")}</CardTitle>
          <StatusChip label={m.user.status} />
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("users.member.email")}>
              {m.user.email ? (
                <a className="hover:underline" href={`mailto:${m.user.email}`}>
                  {m.user.email}
                </a>
              ) : (
                "—"
              )}
            </Field>
            <Field label={t("users.member.phone")}>
              {m.user.phone_e164 ? (
                <a
                  className="hover:underline"
                  href={`tel:${m.user.phone_e164}`}
                  dir="ltr"
                >
                  {m.user.phone_e164}
                </a>
              ) : (
                "—"
              )}
            </Field>
            <Field label={t("users.member.last_login")}>
              {m.user.last_login_at
                ? format.dateTime(m.user.last_login_at)
                : "—"}
            </Field>
          </dl>
          {counters.length > 0 ? (
            <dl
              className="grid gap-4 sm:grid-cols-3 lg:grid-cols-5"
              data-testid="member-counters"
            >
              {counters.map((c) => (
                <Field key={c.key} label={t(`users.member.counts.${c.key}`)}>
                  <span className="text-lg font-semibold">
                    {count(c.value)}
                  </span>
                </Field>
              ))}
            </dl>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("users.member.memberships")}</CardTitle>
        </CardHeader>
        <CardContent>
          {m.memberships.length === 0 ? (
            <p className="text-muted-foreground text-sm">—</p>
          ) : (
            <ul className="divide-y" data-testid="member-memberships">
              {m.memberships.map((ms) => (
                <li
                  key={ms.organization.uuid}
                  className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <EntityLink
                      kind="organization"
                      entity={ms.organization}
                      slug={slug}
                      className="font-medium"
                    />
                    <Badge variant="outline">
                      {t(`users.member.role.${ms.role}`)}
                    </Badge>
                    {ms.role_slugs.map((r) => (
                      <Badge key={r} variant="secondary">
                        {roleLabel(r, t)}
                      </Badge>
                    ))}
                  </div>
                  {ms.warehouses.length > 0 ? (
                    <div className="text-muted-foreground flex flex-wrap gap-x-2 text-xs">
                      <span>{t("users.member.warehouses")}:</span>
                      {ms.warehouses.map((w, i) => (
                        <span key={w.uuid}>
                          <EntityLink kind="warehouse" entity={w} slug={slug} />
                          {i < ms.warehouses.length - 1 ? "," : ""}
                        </span>
                      ))}
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {active ? (
        <Card>
          <CardContent className="pt-6">
            <Tabs value={active} onValueChange={setTab}>
              <TabsList className="flex-wrap">
                {tabs.map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(`users.member.tabs.${value}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
              <TabsContent value="services_created" className="pt-4">
                <RelatedList
                  kind="service"
                  filter={filters.services_created}
                  persistKey="member-services-created-v1"
                  enabled={active === "services_created"}
                />
              </TabsContent>
              <TabsContent value="services_performed" className="pt-4">
                <RelatedList
                  kind="service"
                  filter={filters.services_performed}
                  persistKey="member-services-performed-v1"
                  enabled={active === "services_performed"}
                />
              </TabsContent>
              <TabsContent value="orders" className="pt-4">
                <RelatedList
                  kind="order"
                  filter={filters.orders}
                  persistKey="member-orders-v1"
                  enabled={active === "orders"}
                />
              </TabsContent>
              <TabsContent value="tasks" className="pt-4">
                <RelatedList
                  kind="task"
                  filter={filters.tasks}
                  persistKey="member-tasks-v1"
                  hideColumns={["assignee"]}
                  enabled={active === "tasks"}
                />
              </TabsContent>
              <TabsContent value="leads" className="pt-4">
                <RelatedList
                  kind="lead"
                  filter={filters.leads}
                  persistKey="member-leads-v1"
                  hideColumns={["assignee"]}
                  enabled={active === "leads"}
                />
              </TabsContent>
              {tabs.includes("payroll") ? (
                <TabsContent value="payroll" className="pt-4">
                  {active === "payroll" ? (
                    <PayrollTab slug={slug} userUuid={uuid} />
                  ) : null}
                </TabsContent>
              ) : null}
              {tabs.includes("activity") ? (
                <TabsContent value="activity" className="pt-4">
                  {active === "activity" ? (
                    <UserActivitySection uuid={uuid} />
                  ) : null}
                </TabsContent>
              ) : null}
            </Tabs>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
