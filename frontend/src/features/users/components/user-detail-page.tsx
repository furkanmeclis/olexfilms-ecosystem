"use client";

import { KeyRound, Mail, Pencil, ShieldCheck, ShieldOff } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { UserSetPasswordDialog } from "@/features/users/components/user-set-password-dialog";
import { UserAuthMethodsIcons } from "@/features/users/components/user-auth-methods";
import { StepUpGate } from "@/features/step-up-engine";
import { USER_STATUS_TONE } from "@/features/users/constants";
import {
  useDisableUser,
  useEnableUser,
} from "@/features/users/hooks/use-user-mutations";
import { useUser } from "@/features/users/hooks/use-users-query";
import { userFullName, userInitials } from "@/features/users/lib/user-display";
import { roleDisplayName } from "@/features/roles/lib/role-display";
import type {
  PlatformUserDetail,
  UserStatus,
} from "@/features/users/services/users.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type UserDetailPageProps = {
  uuid: string;
};

function DetailField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs font-medium">{label}</dt>
      <dd className="text-sm break-words">{value ?? "—"}</dd>
    </div>
  );
}

function statusTone(status: string) {
  if (status in USER_STATUS_TONE) {
    return USER_STATUS_TONE[status as UserStatus];
  }
  return "default" as const;
}

function UserDetailActions({
  user,
  onSetPassword,
}: {
  user: PlatformUserDetail;
  onSetPassword: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const enableUser = useEnableUser();
  const disableUser = useDisableUser();

  return (
    <EntityActions>
      <PermissionGuard permission={permissions.users.write}>
        <Button
          type="button"
          size="sm"
          onClick={() => router.push(routes.platform.users.edit(user.uuid))}
        >
          <Pencil className="size-4" />
          {t("users.actions.edit")}
        </Button>
      </PermissionGuard>

      <PermissionGuard permission={permissions.users.write}>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={onSetPassword}
        >
          <KeyRound className="size-4" />
          {t("users.actions.set_password")}
        </Button>
      </PermissionGuard>

      {can(permissions.users.write) &&
      (user.status === "disabled" || user.status === "pending") ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={enableUser.isPending}
          onClick={() => enableUser.mutate(user.uuid)}
        >
          <ShieldCheck className="size-4" />
          {t("users.actions.enable")}
        </Button>
      ) : null}

      {can(permissions.users.write) && user.status === "active" ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disableUser.isPending}
          onClick={() => disableUser.mutate(user.uuid)}
        >
          <ShieldOff className="size-4" />
          {t("users.actions.disable")}
        </Button>
      ) : null}
    </EntityActions>
  );
}

export function UserDetailPage({ uuid }: UserDetailPageProps) {
  const { t } = useLocale();
  const userQuery = useUser(uuid);
  const [passwordOpen, setPasswordOpen] = useState(false);

  const user = userQuery.data;
  const title = user ? userFullName(user) : t("users.detail_title");
  const roles = user?.roles ?? [];

  return (
    <EntityPage
      title={title}
      description={t("users.detail_description")}
      permission={permissions.users.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("users.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("users.title"), href: routes.platform.users.root },
        { label: title },
      ]}
      actions={
        user ? (
          <UserDetailActions
            user={user}
            onSetPassword={() => setPasswordOpen(true)}
          />
        ) : null
      }
    >
      {userQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {userQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("users.detail_not_found")}
          onRetry={() => void userQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {user ? (
        <div className="space-y-6">
          <EntityHeader
            title={userFullName(user)}
            badges={
              <StatusChip
                label={t(`users.status.${user.status}`)}
                tone={statusTone(user.status)}
              />
            }
            leading={
              <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
                <span className="text-muted-foreground text-sm font-medium">
                  {userInitials(user)}
                </span>
              </div>
            }
          >
            <dl className="grid gap-3 sm:grid-cols-2">
              <DetailField
                label={t("users.fields.email")}
                value={
                  <span className="inline-flex items-center gap-1.5">
                    <Mail className="text-muted-foreground size-3.5 shrink-0" />
                    {user.email}
                  </span>
                }
              />
              <DetailField
                label={t("users.fields.uuid")}
                value={<code className="text-xs">{user.uuid}</code>}
              />
            </dl>
          </EntityHeader>

          <EntitySectionCard title={t("users.detail.profile")}>
            <EntityDetail
              sections={[
                {
                  id: "profile",
                  fields: [
                    {
                      key: "name",
                      label: t("users.fields.name"),
                      value: user.name,
                    },
                    {
                      key: "surname",
                      label: t("users.fields.surname"),
                      value: user.surname,
                    },
                    {
                      key: "email",
                      label: t("users.fields.email"),
                      value: user.email,
                    },
                    {
                      key: "status",
                      label: t("users.fields.status"),
                      value: (
                        <StatusChip
                          label={t(`users.status.${user.status}`)}
                          tone={statusTone(user.status)}
                        />
                      ),
                    },
                    {
                      key: "email_verified",
                      label: t("users.fields.email_verified"),
                      value: user.email_verified
                        ? t("users.verified.yes")
                        : t("users.verified.no"),
                    },
                    {
                      key: "uuid",
                      label: t("users.fields.uuid"),
                      value: <code className="text-xs">{user.uuid}</code>,
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard title={t("users.detail.roles")}>
            <StepUpGate purpose="users.detail.roles">
              {roles.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("users.detail.roles_empty")}
                </p>
              ) : (
                <ul className="divide-border divide-y rounded-md border">
                  {roles.map((role) => (
                    <li
                      key={role.uuid}
                      className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5"
                    >
                      <div className="min-w-0 space-y-0.5">
                        <Link
                          href={routes.platform.roles.detail(role.uuid)}
                          className="text-sm font-medium hover:underline"
                        >
                          {roleDisplayName(role, t)}
                        </Link>
                        <p className="text-muted-foreground font-mono text-xs">
                          {role.slug}
                        </p>
                      </div>
                      {role.is_system ? (
                        <Badge variant="outline" className="text-[10px]">
                          {t("roles.labels.system")}
                        </Badge>
                      ) : null}
                    </li>
                  ))}
                </ul>
              )}
            </StepUpGate>
          </EntitySectionCard>

          <EntitySectionCard title={t("users.detail.auth_methods")}>
            {(user.auth_methods ?? []).length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("users.auth_methods.empty")}
              </p>
            ) : (
              <div className="space-y-3">
                <UserAuthMethodsIcons methods={user.auth_methods ?? []} />
                <ul className="divide-border divide-y rounded-md border">
                  {(user.auth_methods ?? []).map((method, index) => (
                    <li
                      key={`${method.kind}-${method.provider ?? method.label ?? index}-${method.linked_at}`}
                      className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5"
                    >
                      <div className="min-w-0 space-y-0.5">
                        <p className="text-sm font-medium">
                          {method.kind === "password"
                            ? t("users.auth_methods.password")
                            : method.kind === "passkey"
                              ? method.label?.trim()
                                ? t("users.auth_methods.passkey_named", {
                                    name: method.label.trim(),
                                  })
                                : t("users.auth_methods.passkey")
                              : (() => {
                                  const provider = method.provider ?? "oauth";
                                  const key = `users.auth_methods.oauth.${provider}`;
                                  const label = t(key);
                                  return label === key
                                    ? t("users.auth_methods.oauth.generic", {
                                        provider,
                                      })
                                    : label;
                                })()}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          {t("users.auth_methods.linked_at", {
                            date: datetime(method.linked_at),
                          })}
                        </p>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </EntitySectionCard>
        </div>
      ) : null}

      <UserSetPasswordDialog
        open={passwordOpen}
        userUuid={user?.uuid ?? null}
        userLabel={user ? userFullName(user) : undefined}
        onOpenChange={setPasswordOpen}
      />
    </EntityPage>
  );
}
