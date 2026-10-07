"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { ConversationPanel } from "@/features/conversations/components/conversation-panel";
import {
  useConversationsColumns,
  type ConversationRowHandlers,
} from "@/features/conversations/components/conversations-columns";
import {
  useConversationsList,
  useConversationsMeta,
  useMarkConversationRead,
  usePlatformAdmins,
  useSetConversationStatus,
} from "@/features/conversations/hooks/use-conversations";
import { useConversationsRealtime } from "@/features/conversations/hooks/use-conversations-realtime";
import type {
  Conversation,
  ConversationListParams,
} from "@/features/conversations/services/conversations.service";
import type { ResourceMeta } from "@/features/io/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CONVERSATIONS_PERSIST_KEY = "platform-conversations-v1";

/**
 * WhatsApp inbox (TEC-399). Platform admin only (S2): the list is an
 * EntityTable; on desktop the selected conversation opens beside it.
 */
export function ConversationsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selected = searchParams.get("c");
  const canRead = can(permissions.conversations.read);
  const canManage = can(permissions.conversations.manage);

  useConversationsRealtime(canRead);

  const select = useCallback(
    (uuid: string | null) => {
      const next = new URLSearchParams(searchParams.toString());
      if (uuid) next.set("c", uuid);
      else next.delete("c");
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  const metaQuery = useConversationsMeta(canRead);
  const meta = metaQuery.data as ResourceMeta | undefined;
  const admins = usePlatformAdmins(canRead);
  const markRead = useMarkConversationRead();
  const setStatus = useSetConversationStatus();

  const handlers = useMemo<ConversationRowHandlers>(
    () => ({
      onOpen: (conversation) => select(conversation.uuid),
      onMarkRead: (conversation) => markRead.mutate(conversation.uuid),
      onToggleClosed: (conversation) =>
        setStatus.mutate({
          uuid: conversation.uuid,
          status: conversation.status === "closed" ? "open" : "closed",
        }),
    }),
    [markRead, select, setStatus],
  );

  const baseColumns = useConversationsColumns({
    handlers,
    assigneeOptions: admins.data ?? [],
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Conversation>(), ...baseColumns],
    [baseColumns],
  );

  const listState = useServerListState({
    columns,
    initialSort: meta?.default_sort ?? "-last_message_at",
    initialPageSize: 20,
    persistKey: CONVERSATIONS_PERSIST_KEY,
  });
  const listParams: ConversationListParams = listState.params;
  const listQuery = useConversationsList(listParams, canRead);
  const items = listQuery.data?.items ?? [];

  const bulkQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: listParams.q,
      sort: listParams.sort,
    }),
    [listState.filterParams, listParams.q, listParams.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery,
    total: listQuery.data?.total ?? 0,
  });
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons("conversations", meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const selectedRow = items.find((item) => item.uuid === selected) ?? null;

  return (
    <EntityPage
      title={t("conversations.title")}
      description={t("conversations.description")}
      permission={permissions.conversations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("conversations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("conversations.title") },
      ]}
    >
      <div
        className={cn(
          "grid gap-4",
          selected &&
            "lg:grid-cols-[minmax(0,1fr)_minmax(22rem,26rem)] xl:grid-cols-[minmax(0,1fr)_minmax(26rem,32rem)]",
        )}
      >
        <div className={cn("min-w-0 space-y-3", selected && "hidden lg:block")}>
          <SelectionBanner
            selectedCount={bulkSelection.selectedCount}
            total={listQuery.data?.total ?? 0}
            showSelectAll={bulkSelection.showSelectAllBanner}
            allMatchingSelected={bulkSelection.scope.mode === "all"}
            onSelectAllMatching={bulkSelection.selectAllMatching}
            onClearSelection={bulkSelection.clearSelection}
          />
          <EntityTable
            columns={columns}
            data={items}
            getRowId={(row) => row.uuid}
            onRowClick={(row) => select(row.uuid)}
            isLoading={listQuery.isLoading}
            isError={listQuery.isError}
            onRetry={() => void listQuery.refetch()}
            emptyTitle={t("conversations.empty_title")}
            emptyDescription={t("conversations.empty_description")}
            rowCount={listQuery.data?.total ?? 0}
            state={{
              ...listState.tableState,
              rowSelection: bulkSelection.rowSelection,
              onRowSelectionChange: bulkSelection.onRowSelectionChange,
            }}
            features={{
              persistKey: CONVERSATIONS_PERSIST_KEY,
              rowSelection: true,
              columnOrdering: true,
              columnPinning: true,
              viewMode: true,
              mobileAutoCards: true,
            }}
            toolbarExtra={
              <>
                {canManage ? (
                  <BulkActionMenu
                    resource="conversations"
                    actions={bulkActions}
                    scope={bulkSelection.scope}
                    selectedCount={bulkSelection.selectedCount}
                    paramOptions={{ assignee_user_uuid: admins.data ?? [] }}
                    onComplete={() => void listQuery.refetch()}
                  />
                ) : null}
                <EntityToolbar
                  onRefresh={() => void listQuery.refetch()}
                  refreshDisabled={listQuery.isFetching}
                />
              </>
            }
          />
        </div>
        {selected ? (
          <div className="h-[calc(100dvh-10rem)] min-h-[28rem] lg:sticky lg:top-4">
            <ConversationPanel
              key={selected}
              uuid={selected}
              initial={selectedRow}
              onClose={() => select(null)}
            />
          </div>
        ) : null}
      </div>
    </EntityPage>
  );
}
