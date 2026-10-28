"use client";

import { Building2, Car, Clock, Monitor, Star, Warehouse } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { ORGANIZATION_STATUS_TONE } from "@/features/organizations/constants";
import type { OrganizationStatus } from "@/features/organizations/services/organizations.service";
import { useUserOverview } from "@/features/users/hooks/use-users-query";
import type {
  UserOverview,
  UserOverviewMembership,
} from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Label of a translation key with a fallback when it is missing. */
function labelOr(t: Translate, key: string, fallback: string) {
  const label = t(key);
  return label === key ? fallback : label;
}

function orgStatusTone(status: string) {
  if (status in ORGANIZATION_STATUS_TONE) {
    return ORGANIZATION_STATUS_TONE[status as OrganizationStatus];
  }
  return "default" as const;
}

function Stat({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value: ReactNode;
}) {
  return (
    <div className="flex items-start gap-3 rounded-md border p-3">
      <div className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
        {icon}
      </div>
      <div className="min-w-0 space-y-0.5">
        <p className="text-muted-foreground text-xs font-medium">{label}</p>
        <div className="text-sm font-medium break-words">{value}</div>
      </div>
    </div>
  );
}

function MembershipCard({
  membership,
}: {
  membership: UserOverviewMembership;
}) {
  const { t, format } = useLocale();
  const org = membership.organization;

  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 space-y-0.5">
          <Link
            href={routes.platform.organizations.detail(org.uuid)}
            className="font-medium hover:underline"
          >
            {org.name}
          </Link>
          <p className="text-muted-foreground text-xs">
            {labelOr(t, `organizations.types.${org.type}`, org.type)}
            {org.parent_name ? ` · ${org.parent_name}` : ""}
            {` · ${org.brand_slug}`}
          </p>
        </div>
        <StatusChip
          label={labelOr(t, `organizations.status.${org.status}`, org.status)}
          tone={orgStatusTone(org.status)}
        />
      </div>
      <dl className="grid gap-2 text-sm sm:grid-cols-2">
        <div className="space-y-1">
          <dt className="text-muted-foreground text-xs font-medium">
            {t("organizations.fields.member_role")}
          </dt>
          <dd>
            {labelOr(
              t,
              `organizations.roles.${membership.role}`,
              membership.role,
            )}
          </dd>
        </div>
        <div className="space-y-1">
          <dt className="text-muted-foreground text-xs font-medium">
            {t("users.overview.member_since")}
          </dt>
          <dd>
            {membership.created_at
              ? format.dateTime(membership.created_at)
              : "—"}
          </dd>
        </div>
      </dl>
      <div className="space-y-1">
        <p className="text-muted-foreground text-xs font-medium">
          {t("organizations.fields.member_roles")}
        </p>
        {membership.role_slugs.length ? (
          <div className="flex flex-wrap gap-1">
            {membership.role_slugs.map((slug) => (
              <Badge key={slug} variant="secondary" title={slug}>
                {labelOr(t, `roles.names.${slug}`, slug)}
              </Badge>
            ))}
          </div>
        ) : (
          <p className="text-sm">—</p>
        )}
      </div>
      <div className="space-y-1">
        <p className="text-muted-foreground text-xs font-medium">
          {t("users.overview.warehouses")}
        </p>
        {membership.warehouses.length ? (
          <ul className="flex flex-wrap gap-1.5">
            {membership.warehouses.map((warehouse) => (
              <li key={warehouse.uuid}>
                <Badge variant="outline" className="gap-1">
                  <Warehouse className="size-3" />
                  {warehouse.name}
                  <span className="text-muted-foreground font-mono">
                    {warehouse.code}
                  </span>
                  {warehouse.is_default ? (
                    <Star
                      className="size-3 fill-current"
                      aria-label={t("users.overview.default_warehouse")}
                    />
                  ) : null}
                </Badge>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("users.overview.warehouses_all")}
          </p>
        )}
      </div>
    </div>
  );
}

function OverviewBody({ overview }: { overview: UserOverview }) {
  const { t, format } = useLocale();
  const customer = overview.customer;

  return (
    <div className="space-y-6">
      <EntitySectionCard title={t("users.overview.summary")}>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Stat
            icon={<Clock className="size-4" />}
            label={t("users.overview.last_login")}
            value={
              overview.last_login_at
                ? format.dateTime(overview.last_login_at)
                : t("users.overview.never")
            }
          />
          <Stat
            icon={<Building2 className="size-4" />}
            label={t("users.overview.organizations")}
            value={overview.memberships.length}
          />
          <Stat
            icon={<Car className="size-4" />}
            label={t("users.overview.vehicles")}
            value={customer.vehicle_count}
          />
          <Stat
            icon={<Monitor className="size-4" />}
            label={t("users.overview.active_sessions")}
            value={overview.sessions.length}
          />
        </div>
      </EntitySectionCard>

      <EntitySectionCard title={t("users.overview.memberships")}>
        {overview.memberships.length ? (
          <div className="grid gap-3 lg:grid-cols-2">
            {overview.memberships.map((membership) => (
              <MembershipCard
                key={membership.organization.uuid}
                membership={membership}
              />
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("users.overview.memberships_empty")}
          </p>
        )}
      </EntitySectionCard>

      <EntitySectionCard title={t("users.overview.customer")}>
        <dl className="grid gap-3 sm:grid-cols-3">
          <div className="space-y-1">
            <dt className="text-muted-foreground text-xs font-medium">
              {t("users.overview.customer_profile")}
            </dt>
            <dd className="text-sm">
              {customer.has_profile
                ? customer.profile_type
                  ? labelOr(
                      t,
                      `users.overview.customer_type.${customer.profile_type}`,
                      customer.profile_type,
                    )
                  : t("users.verified.yes")
                : t("users.overview.customer_none")}
            </dd>
          </div>
          <div className="space-y-1">
            <dt className="text-muted-foreground text-xs font-medium">
              {t("users.overview.company")}
            </dt>
            <dd className="text-sm">{customer.company_name ?? "—"}</dd>
          </div>
          <div className="space-y-1">
            <dt className="text-muted-foreground text-xs font-medium">
              {t("users.overview.vehicles")}
            </dt>
            <dd className="text-sm">{customer.vehicle_count}</dd>
          </div>
        </dl>
        {overview.customer_organizations.length ? (
          <ul className="divide-border mt-4 divide-y rounded-md border">
            {overview.customer_organizations.map((org) => (
              <li
                key={org.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5"
              >
                <div className="min-w-0 space-y-0.5">
                  <Link
                    href={routes.platform.organizations.detail(org.uuid)}
                    className="text-sm font-medium hover:underline"
                  >
                    {org.name}
                  </Link>
                  <p className="text-muted-foreground text-xs">
                    {labelOr(t, `organizations.types.${org.type}`, org.type)}
                    {` · ${org.brand_slug}`}
                  </p>
                </div>
                <p className="text-muted-foreground text-xs">
                  {org.first_service_at
                    ? t("users.overview.first_service", {
                        date: format.dateTime(org.first_service_at),
                      })
                    : null}
                </p>
              </li>
            ))}
          </ul>
        ) : null}
      </EntitySectionCard>

      <EntitySectionCard title={t("users.overview.sessions")}>
        {overview.sessions.length ? (
          <ul className="divide-border divide-y rounded-md border">
            {overview.sessions.map((session) => (
              <li
                key={session.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="text-sm font-medium">
                    {labelOr(
                      t,
                      `users.overview.realm.${session.realm}`,
                      session.realm,
                    )}
                    {session.device_name ? ` · ${session.device_name}` : ""}
                    {session.organization_name
                      ? ` · ${session.organization_name}`
                      : ""}
                  </p>
                  <p className="text-muted-foreground truncate text-xs">
                    {[session.ip_address, session.user_agent]
                      .filter(Boolean)
                      .join(" · ") || "—"}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {session.impersonated ? (
                    <Badge variant="outline">
                      {t("users.overview.impersonated")}
                    </Badge>
                  ) : null}
                  <p className="text-muted-foreground text-xs">
                    {session.created_at
                      ? t("users.overview.session_started", {
                          date: format.dateTime(session.created_at),
                        })
                      : null}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("users.overview.sessions_empty")}
          </p>
        )}
      </EntitySectionCard>
    </div>
  );
}

/**
 * TEC-1036: user 360 on the platform user detail — every organization the
 * user belongs to (kind, roles, warehouses) with links, the customer side,
 * last login and active sessions. Global roles stay in the step-up gated
 * "Roles" card of the same page.
 */
export function UserOverviewCards({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const query = useUserOverview(uuid);

  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("users.overview.error")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  return <OverviewBody overview={query.data} />;
}
