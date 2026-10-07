"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { CheckCircle2, XCircle } from "lucide-react";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { actionLabel } from "@/features/ai-assistant/lib/labels";
import type { ActionPreview } from "@/features/ai-assistant/lib/types";
import {
  formatCountdown,
  PendingActionDialog,
  useSecondsLeft,
} from "@/features/mcp/components/pending-action-dialog";
import {
  useDecidePendingAction,
  usePendingActions,
} from "@/features/mcp/hooks/use-mcp";
import {
  APPROVAL_SOURCES,
  type AIActionCard,
  type PendingActionListParams,
} from "@/features/mcp/services/mcp.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const PENDING_ACTIONS_PERSIST_KEY = "tenant-ai-pending-actions-v1";

function RemainingTime({ expiresAt }: { expiresAt: string }) {
  const { t } = useLocale();
  const seconds = useSecondsLeft(expiresAt);
  return seconds > 0 ? (
    <span className="text-sm tabular-nums">{formatCountdown(seconds)}</span>
  ) : (
    <Badge variant="outline" className="font-normal">
      {t("mcp.approvals.expired_short")}
    </Badge>
  );
}

/**
 * "Pending AI actions" (TEC-403, F4-03d): write tools an MCP client or
 * WhatsApp proposed for the signed-in user in this organization. A row (or
 * the `?action=<uuid>` link the MCP tool result carries) opens the
 * confirmation card; confirm / cancel go to /v1/ai/pending-actions.
 */
export function PendingActionsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canConfirm = can(permissions.ai.actionsConfirm);
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selected = searchParams.get("action");
  const decide = useDecidePendingAction();

  const select = useCallback(
    (uuid: string | null) => {
      const next = new URLSearchParams(searchParams.toString());
      if (uuid) next.set("action", uuid);
      else next.delete("action");
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  const { mutate } = decide;
  const decideAction = useCallback(
    (action: AIActionCard, confirm: boolean) =>
      mutate(
        { uuid: action.action_uuid, confirm },
        {
          onSuccess: (out) => {
            if (!confirm) appToast.success(t("mcp.approvals.rejected"));
            else if (out.status === "confirmed") {
              appToast.success(t("mcp.approvals.confirmed"));
            } else {
              appToast.error(out.message || t("mcp.approvals.failed"));
            }
            select(null);
          },
          onError: (error) => {
            const code = isApiError(error) ? error.code : "";
            appToast.error(
              code === "AI_ACTION_EXPIRED"
                ? t("mcp.approvals.expired")
                : code === "AI_ACTION_RESOLVED"
                  ? t("mcp.approvals.resolved")
                  : t("common.error_generic"),
            );
            select(null);
          },
        },
      ),
    [mutate, select, t],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<AIActionCard>({
          accessorKey: "tool_name",
          labelKey: "mcp.approvals.tool",
          enableSorting: true,
          enableColumnFilter: false,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const preview = (row.original.preview ?? {}) as ActionPreview;
            return (
              <div className="flex min-w-0 flex-col">
                <span className="truncate font-medium">
                  {actionLabel(
                    t,
                    preview.action ?? row.original.tool_name,
                    preview.summary,
                  )}
                </span>
                <span
                  className="text-muted-foreground truncate font-mono text-xs"
                  dir="ltr"
                >
                  {row.original.tool_name}
                </span>
              </div>
            );
          },
        }),
        createColumn<AIActionCard>({
          accessorKey: "source",
          labelKey: "mcp.approvals.source",
          enableSorting: false,
          filterVariant: "faceted",
          param: "source",
          filterOptions: APPROVAL_SOURCES.map((value) => ({
            value,
            labelKey: `mcp.source.${value}`,
            label: value,
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant="outline" className="font-normal">
              {t(`mcp.source.${row.original.source}`)}
            </Badge>
          ),
        }),
        createColumn<AIActionCard>({
          id: "summary",
          accessorFn: (row) => (row.preview as ActionPreview)?.summary ?? "",
          labelKey: "mcp.approvals.summary",
          // Searched through the toolbar `q` (tool name or summary).
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">
              {(row.original.preview as ActionPreview)?.summary ?? "—"}
            </span>
          ),
        }),
        createColumn<AIActionCard>({
          accessorKey: "expires_at",
          labelKey: "mcp.approvals.remaining",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <RemainingTime expiresAt={row.original.expires_at} />
          ),
        }),
        createColumn<AIActionCard>({
          accessorKey: "created_at",
          labelKey: "mcp.approvals.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span
              className="text-sm tabular-nums"
              title={format.dateTime(row.original.created_at)}
            >
              {format.relative(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<AIActionCard>({
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
                  id: "review",
                  label: t("mcp.approvals.review"),
                  icon: CheckCircle2,
                  onSelect: () => select(row.original.action_uuid),
                },
                {
                  id: "reject",
                  label: t("mcp.approvals.reject"),
                  icon: XCircle,
                  variant: "destructive",
                  onSelect: () => decideAction(row.original, false),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AIActionCard, unknown>[],
    [decideAction, format, select, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: PENDING_ACTIONS_PERSIST_KEY,
  });
  const params: PendingActionListParams = listState.params;
  const list = usePendingActions(params, canConfirm);
  const items = list.data?.items ?? [];
  const current = items.find((a) => a.action_uuid === selected) ?? null;

  return (
    <EntityPage
      title={t("mcp.approvals.title")}
      description={t("mcp.approvals.description")}
      permission={permissions.ai.actionsConfirm}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("mcp.approvals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("mcp.approvals.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.action_uuid}
        onRowClick={(row) => select(row.action_uuid)}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("mcp.approvals.empty_title")}
        emptyDescription={t("mcp.approvals.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: PENDING_ACTIONS_PERSIST_KEY,
          columnOrdering: true,
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
      <PendingActionDialog
        action={current}
        pending={decide.isPending}
        onConfirm={(action) => decideAction(action, true)}
        onCancel={(action) => decideAction(action, false)}
        onClose={() => select(null)}
      />
    </EntityPage>
  );
}
