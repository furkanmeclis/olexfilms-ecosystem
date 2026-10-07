"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useMcpClients,
  useRevokeMcpClient,
} from "@/features/mcp/hooks/use-mcp";
import {
  CLIENT_STATUSES,
  type ClientListParams,
  type OAuthClientSummary,
} from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const MCP_CLIENTS_PERSIST_KEY = "platform-mcp-clients-v1";

/**
 * Registered MCP clients (TEC-403, super_admin / mcp.clients.manage):
 * client, registration IP, connected users, last use. "Revoke" blocks the
 * client and ends every connection of it.
 */
export function McpClientsPage() {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.mcp.clientsManage);
  const [target, setTarget] = useState<OAuthClientSummary | null>(null);
  const revoke = useRevokeMcpClient();

  const columns = useMemo(
    () =>
      [
        createColumn<OAuthClientSummary>({
          accessorKey: "client_name",
          labelKey: "mcp.clients.client",
          enableSorting: true,
          enableColumnFilter: false,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span className="truncate font-medium">
                {row.original.client_name}
              </span>
              <span
                className="text-muted-foreground truncate font-mono text-xs"
                dir="ltr"
              >
                {row.original.client_id}
              </span>
            </div>
          ),
        }),
        createColumn<OAuthClientSummary>({
          accessorKey: "status",
          labelKey: "mcp.clients.status",
          enableSorting: false,
          filterVariant: "faceted",
          param: "status",
          filterOptions: CLIENT_STATUSES.map((value) => ({
            value,
            labelKey: `mcp.clients.status_value.${value}`,
            label: value,
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`mcp.clients.status_value.${row.original.status}`)}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<OAuthClientSummary>({
          accessorKey: "created_ip",
          labelKey: "mcp.clients.created_ip",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="font-mono text-sm" dir="ltr">
              {row.original.created_ip ?? "—"}
            </span>
          ),
        }),
        createColumn<OAuthClientSummary>({
          id: "redirect_hosts",
          accessorFn: (row) => row.redirect_uris.join(" "),
          labelKey: "mcp.clients.redirect_uris",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs break-all" dir="ltr">
              {row.original.redirect_uris.join(", ")}
            </span>
          ),
        }),
        createColumn<OAuthClientSummary>({
          accessorKey: "active_grants",
          labelKey: "mcp.clients.users",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.number(row.original.active_grants)}
            </span>
          ),
        }),
        createColumn<OAuthClientSummary>({
          accessorKey: "created_at",
          labelKey: "mcp.clients.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<OAuthClientSummary>({
          accessorKey: "last_used_at",
          labelKey: "mcp.clients.last_used_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.last_used_at ? (
              <span
                className="text-sm tabular-nums"
                title={format.dateTime(row.original.last_used_at)}
              >
                {format.relative(row.original.last_used_at)}
              </span>
            ) : (
              <span className="text-muted-foreground text-sm">—</span>
            ),
        }),
        createColumn<OAuthClientSummary>({
          id: "actions",
          labelKey: "mcp.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "revoke",
                  label: t("mcp.clients.revoke"),
                  icon: Ban,
                  variant: "destructive",
                  permission: permissions.mcp.clientsManage,
                  disabled: row.original.status === "revoked",
                  onSelect: () => setTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<OAuthClientSummary, unknown>[],
    [format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: MCP_CLIENTS_PERSIST_KEY,
  });
  const params: ClientListParams = listState.params;
  const list = useMcpClients(params, canManage);

  const confirmRevoke = () => {
    if (!target) return;
    revoke.mutate(target.uuid, {
      onSuccess: () => {
        appToast.success(t("mcp.clients.revoked"));
        setTarget(null);
      },
      onError: () => appToast.error(t("common.error_generic")),
    });
  };

  return (
    <EntityPage
      title={t("mcp.clients.title")}
      description={t("mcp.clients.description")}
      permission={permissions.mcp.clientsManage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("mcp.clients.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("mcp.clients.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("mcp.clients.empty_title")}
        emptyDescription={t("mcp.clients.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: MCP_CLIENTS_PERSIST_KEY,
          columnOrdering: true,
          columnPinning: true,
          viewMode: true,
          mobileAutoCards: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <ConfirmDialog
        open={target !== null}
        title={t("mcp.clients.revoke_title")}
        description={
          target
            ? t("mcp.clients.revoke_body", {
                client: target.client_name,
                count: target.active_grants,
              })
            : undefined
        }
        confirmLabel={t("mcp.clients.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onConfirm={confirmRevoke}
        onCancel={() => setTarget(null)}
      />
    </EntityPage>
  );
}
