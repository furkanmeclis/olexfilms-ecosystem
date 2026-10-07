"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Unplug } from "lucide-react";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { useGrants, useRevokeGrant } from "@/features/mcp/hooks/use-mcp";
import {
  MCP_REALMS,
  type GrantListParams,
  type OAuthGrant,
  type SessionRealm,
} from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const CONNECTED_APPS_PERSIST_KEY = {
  panel: "profile-connected-apps-v1",
  portal: "portal-connected-apps-v1",
} as const;

/**
 * "Connected apps" (TEC-403): the caller's MCP grants (AI clients such as
 * Claude or ChatGPT) with sort, `q` and the endpoint filter; "Disconnect"
 * asks first, then DELETEs the grant (its tokens stop at once).
 */
export function ConnectedAppsTable({ realm }: { realm: SessionRealm }) {
  const { t, format } = useLocale();
  const [target, setTarget] = useState<OAuthGrant | null>(null);
  const revoke = useRevokeGrant(realm);

  const columns = useMemo(
    () =>
      [
        createColumn<OAuthGrant>({
          accessorKey: "client_name",
          labelKey: "mcp.grants.client",
          enableSorting: true,
          enableColumnFilter: false,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.client_name}</span>
          ),
        }),
        createColumn<OAuthGrant>({
          accessorKey: "realm",
          labelKey: "mcp.grants.endpoint",
          enableSorting: false,
          // The customer portal only has the customer endpoint.
          enableColumnFilter: realm === "panel",
          filterVariant: "faceted",
          param: "realm",
          filterOptions: MCP_REALMS.filter((r) => r !== "customer").map(
            (value) => ({
              value,
              labelKey: `mcp.realm.${value}`,
              label: value,
            }),
          ),
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant="outline" className="font-normal" dir="ltr">
              {row.original.resource}
            </Badge>
          ),
        }),
        createColumn<OAuthGrant>({
          accessorKey: "organization_name",
          labelKey: "mcp.grants.organization",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm">{row.original.organization_name}</span>
          ),
        }),
        createColumn<OAuthGrant>({
          accessorKey: "created_at",
          labelKey: "mcp.grants.connected_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<OAuthGrant>({
          accessorKey: "last_used_at",
          labelKey: "mcp.grants.last_used_at",
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
              <span className="text-muted-foreground text-sm">
                {t("mcp.grants.never_used")}
              </span>
            ),
        }),
        createColumn<OAuthGrant>({
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
                  id: "disconnect",
                  label: t("mcp.grants.disconnect"),
                  icon: Unplug,
                  variant: "destructive",
                  onSelect: () => setTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<OAuthGrant, unknown>[],
    [format, realm, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 10,
    persistKey: CONNECTED_APPS_PERSIST_KEY[realm],
  });
  const params: GrantListParams = listState.params;
  const list = useGrants(realm, params);

  const disconnect = () => {
    if (!target) return;
    revoke.mutate(target.uuid, {
      onSuccess: () => {
        appToast.success(t("mcp.grants.disconnected"));
        setTarget(null);
      },
      onError: () => appToast.error(t("common.error_generic")),
    });
  };

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("mcp.grants.empty_title")}
        emptyDescription={t("mcp.grants.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: CONNECTED_APPS_PERSIST_KEY[realm],
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
        title={t("mcp.grants.disconnect_title")}
        description={
          target
            ? t("mcp.grants.disconnect_body", {
                client: target.client_name,
                organization: target.organization_name,
              })
            : undefined
        }
        confirmLabel={t("mcp.grants.disconnect")}
        variant="destructive"
        isPending={revoke.isPending}
        onConfirm={disconnect}
        onCancel={() => setTarget(null)}
      />
    </>
  );
}
