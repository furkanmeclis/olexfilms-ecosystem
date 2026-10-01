"use client";

import { Play, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { PurgeRuleFormDrawer } from "@/features/logs/components/purge-rule-form-drawer";
import {
  useCreatePurgeRule,
  useDeletePurgeRule,
  useRunPurgeRule,
  useUpdatePurgeRule,
} from "@/features/logs/hooks/use-log-mutations";
import { logsKeys } from "@/features/logs/hooks/query-keys";
import type { PurgeRuleFormValues } from "@/features/logs/schemas/purge-rule-form";
import {
  logsService,
  type PurgeRule,
} from "@/features/logs/services/logs.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

function intervalLabel(t: (key: string) => string, minutes: number) {
  const key = `logs.rules.intervals.${minutes}`;
  const label = t(key);
  return label === key ? `${minutes}m` : label;
}

export function PurgeRulesPanel() {
  const { t, format } = useLocale();
  const { confirmDelete } = useDialogs();
  const listState = useServerListState({ initialPageSize: 50 });
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<PurgeRule | null>(null);

  const rulesQuery = useQuery({
    queryKey: logsKeys.rules(),
    queryFn: () => logsService.listRules(),
  });

  const createRule = useCreatePurgeRule();
  const updateRule = useUpdatePurgeRule();
  const deleteRule = useDeletePurgeRule();
  const runRule = useRunPurgeRule();

  const openCreate = useCallback(() => {
    setEditing(null);
    setEditorOpen(true);
  }, []);

  const openEdit = useCallback((rule: PurgeRule) => {
    setEditing(rule);
    setEditorOpen(true);
  }, []);

  const handleDelete = useCallback(
    async (rule: PurgeRule) => {
      const confirmed = await confirmDelete({
        title: t("logs.rules.delete_title"),
        description: t("logs.rules.delete_description", { name: rule.name }),
      });
      if (!confirmed) return;
      await deleteRule.mutateAsync(rule.uuid);
    },
    [confirmDelete, deleteRule, t],
  );

  const columns = useMemo<ColumnDef<PurgeRule>[]>(
    () => [
      createColumn<PurgeRule>({
        accessorKey: "name",
        labelKey: "logs.rules.columns.name",
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{row.original.name}</span>
            {row.original.is_system ? (
              <Badge variant="secondary" className="text-[10px]">
                {t("logs.rules.system_badge")}
              </Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<PurgeRule>({
        accessorKey: "enabled",
        labelKey: "logs.rules.columns.enabled",
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.enabled
                ? t("logs.rules.enabled_yes")
                : t("logs.rules.enabled_no")
            }
            tone={row.original.enabled ? "success" : "default"}
          />
        ),
      }),
      createColumn<PurgeRule>({
        accessorKey: "levels",
        labelKey: "logs.rules.columns.levels",
        cell: ({ row }) =>
          row.original.levels
            .map((level) => t(`logs.levels.${level}`))
            .join(", "),
      }),
      createColumn<PurgeRule>({
        accessorKey: "older_than_hours",
        labelKey: "logs.rules.columns.older_than",
        cell: ({ row }) =>
          t("logs.rules.older_than_value", {
            hours: row.original.older_than_hours,
          }),
      }),
      createColumn<PurgeRule>({
        accessorKey: "interval_minutes",
        labelKey: "logs.rules.columns.schedule",
        cell: ({ row }) => intervalLabel(t, row.original.interval_minutes),
      }),
      createColumn<PurgeRule>({
        accessorKey: "last_run_at",
        labelKey: "logs.rules.columns.last_run",
        cell: ({ row }) =>
          row.original.last_run_at
            ? format.dateTime(row.original.last_run_at)
            : "—",
      }),
      createColumn<PurgeRule>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "run",
                label: t("logs.rules.run_now"),
                icon: Play,
                permission: permissions.logs.write,
                onSelect: () => void runRule.mutateAsync(row.original.uuid),
              },
              {
                id: "edit",
                label: t("common.save"),
                permission: permissions.logs.write,
                onSelect: () => openEdit(row.original),
              },
              ...(!row.original.is_system
                ? [
                    {
                      id: "delete",
                      label: t("common.delete"),
                      icon: Trash2,
                      variant: "destructive" as const,
                      permission: permissions.logs.write,
                      onSelect: () => void handleDelete(row.original),
                    },
                  ]
                : []),
            ]}
          />
        ),
      }),
    ],
    [handleDelete, openEdit, runRule, t, format],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(
      (rulesQuery.data?.items.length ?? 0) /
        (listState.pagination.pageSize || 50),
    ),
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={rulesQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={rulesQuery.isLoading}
        isError={rulesQuery.isError}
        onRetry={() => void rulesQuery.refetch()}
        emptyTitle={t("logs.rules.empty_title")}
        emptyDescription={t("logs.rules.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "platform-log-rules-v1", rowSelection: false }}
        toolbarExtra={
          <>
            <EntityCreateButton
              onClick={openCreate}
              label={t("logs.rules.create")}
              permission={permissions.logs.write}
            />
            <EntityToolbar
              onRefresh={() => void rulesQuery.refetch()}
              refreshDisabled={rulesQuery.isFetching}
            />
          </>
        }
      />

      <PurgeRuleFormDrawer
        open={editorOpen}
        onOpenChange={setEditorOpen}
        rule={editing}
        isSubmitting={createRule.isPending || updateRule.isPending}
        onSubmit={async (values: PurgeRuleFormValues) => {
          const body = {
            name: values.name,
            enabled: values.enabled,
            levels: values.levels,
            source: values.source?.trim() || undefined,
            message_contains: values.message_contains?.trim() || undefined,
            older_than_hours: values.older_than_hours,
            interval_minutes: Number(values.interval_minutes),
          };
          if (editing) {
            await updateRule.mutateAsync({ uuid: editing.uuid, body });
          } else {
            await createRule.mutateAsync(body);
          }
        }}
      />
    </>
  );
}
