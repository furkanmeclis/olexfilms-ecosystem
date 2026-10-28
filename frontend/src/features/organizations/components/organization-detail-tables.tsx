"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ShieldCheck, UserMinus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions, EntityTable } from "@/components/entity";
import { createColumn, type DataTableManual } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ORGANIZATION_MEMBER_ROLES,
  ORGANIZATION_STATUS_TONE,
  ORGANIZATION_STATUS_VALUES,
  ORGANIZATION_TYPE_VALUES,
} from "@/features/organizations/constants";
import { useOrganizationChildren } from "@/features/organizations/hooks/use-organizations-query";
import type {
  Organization,
  OrganizationMember,
  OrganizationStatus,
} from "@/features/organizations/services/organizations.service";
import {
  USER_STATUS_TONE,
  USER_STATUS_VALUES,
} from "@/features/users/constants";
import { userFullName } from "@/features/users/lib/user-display";
import type { UserStatus } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

/** Nested lists hold the full array → client-side sort/filter/paging. */
export const CLIENT_SIDE: DataTableManual = {
  sorting: false,
  filtering: false,
  pagination: false,
};

export const ORGANIZATION_MEMBERS_PERSIST_KEY =
  "platform-organization-members-v2";
export const ORGANIZATION_CHILDREN_PERSIST_KEY =
  "platform-organization-children-v1";

function memberStatusTone(status: string) {
  if (status in USER_STATUS_TONE) {
    return USER_STATUS_TONE[status as UserStatus];
  }
  return "default" as const;
}

function organizationStatusTone(status: string) {
  if (status in ORGANIZATION_STATUS_TONE) {
    return ORGANIZATION_STATUS_TONE[status as OrganizationStatus];
  }
  return "default" as const;
}

/** Display name of a system role slug (falls back to the slug). */
function roleLabel(slug: string, t: (key: string) => string) {
  const key = `roles.names.${slug}`;
  const label = t(key);
  return label === key ? slug : label;
}

export type OrganizationMemberActionHandlers = {
  /** TEC-1036: fix the membership kind / organization roles. */
  onEditRoles: (member: OrganizationMember) => void;
  /** TEC-1036: remove the member from the organization (confirmed). */
  onRemove: (member: OrganizationMember) => void;
};

/** Staff of an organization (embedded in `GET /organizations/{uuid}`). */
export function OrganizationMembersTable({
  members,
  handlers,
}: {
  members: OrganizationMember[];
  handlers?: OrganizationMemberActionHandlers;
}) {
  const { t, format } = useLocale();
  const router = useRouter();
  const roleSlugOptions = useMemo(
    () =>
      [...new Set(members.flatMap((member) => member.role_slugs ?? []))]
        .sort()
        .map((value) => ({ value, label: roleLabel(value, t) })),
    [members, t],
  );

  const columns = useMemo<ColumnDef<OrganizationMember, unknown>[]>(
    () => [
      createColumn<OrganizationMember>({
        id: "name",
        accessorFn: (row) => userFullName(row),
        labelKey: "organizations.fields.member",
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.platform.users.detail(row.original.uuid)}
            className="font-medium hover:underline"
          >
            {userFullName(row.original)}
          </Link>
        ),
      }),
      createColumn<OrganizationMember>({
        accessorKey: "email",
        labelKey: "organizations.columns.email",
        gridSecondary: true,
      }),
      createColumn<OrganizationMember>({
        accessorKey: "role",
        labelKey: "organizations.fields.member_role",
        filterVariant: "faceted",
        filterOptions: ORGANIZATION_MEMBER_ROLES.map((value) => ({
          value,
          labelKey: `organizations.roles.${value}`,
          label: value,
        })),
        cell: ({ row }) => t(`organizations.roles.${row.original.role}`),
      }),
      createColumn<OrganizationMember>({
        id: "role_slugs",
        accessorFn: (row) => row.role_slugs ?? [],
        labelKey: "organizations.fields.member_roles",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: roleSlugOptions,
        filterFn: (row, columnId, filterValue: unknown) => {
          const selected = filterValue as string[] | undefined;
          if (!selected?.length) return true;
          const slugs = row.getValue<string[]>(columnId);
          return selected.some((slug) => slugs.includes(slug));
        },
        cell: ({ row }) => {
          const slugs = row.original.role_slugs ?? [];
          if (!slugs.length) return "—";
          return (
            <div className="flex flex-wrap gap-1">
              {slugs.map((slug) => (
                <Badge key={slug} variant="secondary" title={slug}>
                  {roleLabel(slug, t)}
                </Badge>
              ))}
            </div>
          );
        },
      }),
      createColumn<OrganizationMember>({
        accessorKey: "status",
        labelKey: "organizations.columns.status",
        filterVariant: "faceted",
        filterOptions: USER_STATUS_VALUES.map((value) => ({
          value,
          labelKey: `users.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(`users.status.${row.original.status}`)}
            tone={memberStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<OrganizationMember>({
        accessorKey: "created_at",
        labelKey: "organizations.columns.created_at",
        cell: ({ row }) =>
          row.original.created_at
            ? format.dateTime(row.original.created_at)
            : "—",
      }),
      ...(handlers
        ? [
            createColumn<OrganizationMember>({
              id: "actions",
              labelKey: "organizations.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              enableColumnFilter: false,
              cell: ({ row }) => (
                <EntityRowActions
                  actions={[
                    {
                      id: "edit_roles",
                      label: t("organizations.actions.edit_member_roles"),
                      icon: ShieldCheck,
                      permission: permissions.organizations.write,
                      onSelect: () => handlers.onEditRoles(row.original),
                    },
                    {
                      id: "remove",
                      label: t("organizations.actions.remove_member"),
                      icon: UserMinus,
                      variant: "destructive",
                      permission: permissions.organizations.write,
                      onSelect: () => handlers.onRemove(row.original),
                    },
                  ]}
                />
              ),
            }),
          ]
        : []),
    ],
    [format, handlers, roleSlugOptions, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={members}
      getRowId={(row) => row.uuid}
      onRowClick={(member) =>
        router.push(routes.platform.users.detail(member.uuid))
      }
      manual={CLIENT_SIDE}
      emptyTitle={t("organizations.detail.members_empty")}
      emptyDescription=""
      features={{
        persistKey: ORGANIZATION_MEMBERS_PERSIST_KEY,
        rowSelection: false,
      }}
    />
  );
}

/** Direct sub-organizations (`GET /organizations/{uuid}/children`). */
export function OrganizationChildrenTable({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const query = useOrganizationChildren(uuid);

  const columns = useMemo<ColumnDef<Organization, unknown>[]>(
    () => [
      createColumn<Organization>({
        accessorKey: "name",
        labelKey: "organizations.columns.name",
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.platform.organizations.detail(row.original.uuid)}
            className="font-medium hover:underline"
          >
            {row.original.name}
          </Link>
        ),
      }),
      createColumn<Organization>({
        accessorKey: "slug",
        labelKey: "organizations.columns.slug",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            /{row.original.slug}
          </span>
        ),
      }),
      createColumn<Organization>({
        accessorKey: "type",
        labelKey: "organizations.columns.type",
        filterVariant: "faceted",
        filterOptions: ORGANIZATION_TYPE_VALUES.map((value) => ({
          value,
          labelKey: `organizations.types.${value}`,
          label: value,
        })),
        cell: ({ row }) =>
          row.original.type
            ? t(`organizations.types.${row.original.type}`)
            : "—",
      }),
      createColumn<Organization>({
        accessorKey: "city",
        labelKey: "organizations.columns.city",
      }),
      createColumn<Organization>({
        accessorKey: "status",
        labelKey: "organizations.columns.status",
        filterVariant: "faceted",
        filterOptions: ORGANIZATION_STATUS_VALUES.map((value) => ({
          value,
          labelKey: `organizations.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(`organizations.status.${row.original.status}`)}
            tone={organizationStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<Organization>({
        accessorKey: "access_ends_at",
        labelKey: "organizations.columns.access_ends_at",
        cell: ({ row }) =>
          row.original.access_ends_at
            ? format.dateTime(row.original.access_ends_at)
            : "—",
      }),
      createColumn<Organization>({
        accessorKey: "created_at",
        labelKey: "organizations.columns.created_at",
        cell: ({ row }) =>
          row.original.created_at
            ? format.dateTime(row.original.created_at)
            : "—",
      }),
    ],
    [format, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(organization) =>
        router.push(routes.platform.organizations.detail(organization.uuid))
      }
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      manual={CLIENT_SIDE}
      emptyTitle={t("organizations.detail.children_empty")}
      emptyDescription=""
      features={{
        persistKey: ORGANIZATION_CHILDREN_PERSIST_KEY,
        rowSelection: false,
      }}
    />
  );
}
