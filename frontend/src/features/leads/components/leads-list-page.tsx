"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, ListChecks, UserPlus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { ExportMenu } from "@/features/io/components/export-menu";
import {
  LEAD_PAGE_SIZE,
  LEAD_SOURCES,
  LEAD_STATUSES,
  LEAD_TARGET_TYPES,
  LEAD_TEMPERATURES,
  leadName,
  leadStatusTone,
  leadTemperatureTone,
} from "@/features/leads/lib/leads";
import {
  leadKeys,
  leadsService,
  type Lead,
  type LeadListQuery,
} from "@/features/leads/services/leads.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const LEADS_PERSIST_KEY = "tenant-leads-v1";
/** Lead list export (TEC-371, ioengine resource `leads.list`). */
export const LEADS_EXPORT_PATH = "/v1/leads/export";
/** `assignee_user_id` filter value for unassigned leads (TEC-371). */
export const LEAD_ASSIGNEE_NONE = "none";

type FollowUp = "" | "overdue" | "today";
const FOLLOW_UPS: FollowUp[] = ["", "overdue", "today"];

/**
 * `POST /v1/leads/bulk` (TEC-371): assign (internal user id) and
 * set_status (pipeline rules per item; lost needs a reason). Both are
 * undoable; ids or "select all matching" with the list filters.
 */
export const LEAD_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "assign",
    label_key: "bulk.actions.leads.assign",
    permission: permissions.leads.write,
    reversible: true,
    params: [
      {
        key: "assignee_user_id",
        kind: "number",
        required: true,
        label_key: "bulk.params.assignee",
      },
    ],
    icon: UserPlus,
  },
  {
    id: "set_status",
    label_key: "bulk.actions.leads.set_status",
    permission: permissions.leads.write,
    reversible: true,
    confirm_key: "bulk.confirm.leads.set_status",
    params: [
      {
        key: "status",
        kind: "enum",
        required: true,
        label_key: "bulk.params.lead_status",
        options: LEAD_STATUSES,
      },
      {
        key: "lost_reason",
        kind: "text",
        label_key: "bulk.params.lost_reason",
      },
    ],
    icon: ListChecks,
  },
];

function enumOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/** Selected values of a faceted column filter. */
function selectedValues(
  filters: { id: string; value: unknown }[],
  id: string,
): string[] {
  const value = filters.find((f) => f.id === id)?.value;
  return Array.isArray(value) ? value.map(String) : [];
}

/**
 * Tenant > Leads (TEC-288, TEC-372): server DataTable over GET /v1/leads
 * with sort, status / target / source / temperature facets, an assignee
 * filter (including unassigned), created range and `q`; the follow-up
 * tabs stay in the toolbar. Writers select rows (or every matching lead)
 * for bulk assign / status; the export uses the same filters and sort.
 */
export function LeadsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canRead = can(permissions.leads.read);
  const canWrite = can(permissions.leads.write);
  const [followUp, setFollowUp] = useState<FollowUp>("");
  const [items, setItems] = useState<Lead[]>([]);

  // The API has no user directory with internal ids: offer "unassigned"
  // plus the assignees on the page and the ones already chosen.
  const [assigneeFilter, setAssigneeFilter] = useState<string[]>([]);
  const assigneeOptions = useMemo(() => {
    const ids = new Set<string>();
    for (const lead of items) {
      if (lead.assignee_user_id) ids.add(String(lead.assignee_user_id));
    }
    for (const id of assigneeFilter) {
      if (id !== LEAD_ASSIGNEE_NONE) ids.add(id);
    }
    return [
      {
        value: LEAD_ASSIGNEE_NONE,
        label: t("leads.form.unassigned"),
      },
      ...[...ids]
        .sort((a, b) => Number(a) - Number(b))
        .map((id) => ({ value: id, label: `#${id}` })),
    ];
  }, [assigneeFilter, items, t]);

  const baseColumns = useMemo(
    () =>
      [
        createColumn<Lead>({
          accessorKey: "target_type",
          labelKey: "leads.columns.target_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumOptions(LEAD_TARGET_TYPES, "leads.target_type"),
          param: "target_type",
          cell: ({ row }) => t(`leads.target_type.${row.original.target_type}`),
        }),
        createColumn<Lead>({
          id: "name",
          accessorFn: (row) => leadName(row),
          labelKey: "leads.columns.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.leads.detail(slug, row.original.uuid)}
              className="font-medium hover:underline"
              data-testid="lead-row"
              onClick={(event) => event.stopPropagation()}
            >
              {leadName(row.original)}
            </Link>
          ),
        }),
        createColumn<Lead>({
          accessorKey: "source",
          labelKey: "leads.columns.source",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumOptions(LEAD_SOURCES, "leads.source"),
          param: "source",
          cell: ({ row }) => t(`leads.source.${row.original.source}`),
        }),
        createColumn<Lead>({
          accessorKey: "temperature",
          labelKey: "leads.columns.temperature",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(LEAD_TEMPERATURES, "leads.temperature"),
          param: "temperature",
          cell: ({ row }) => (
            <StatusChip
              label={t(`leads.temperature.${row.original.temperature}`)}
              tone={leadTemperatureTone(row.original.temperature)}
            />
          ),
        }),
        createColumn<Lead>({
          accessorKey: "status",
          labelKey: "leads.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(LEAD_STATUSES, "leads.status"),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`leads.status.${row.original.status}`)}
              tone={leadStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Lead>({
          accessorKey: "follow_up_date",
          labelKey: "leads.columns.follow_up",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {row.original.follow_up_date
                ? format.dateTime(row.original.follow_up_date)
                : "—"}
            </span>
          ),
        }),
        createColumn<Lead>({
          id: "assignee",
          accessorFn: (row) =>
            row.assignee_user_id
              ? String(row.assignee_user_id)
              : LEAD_ASSIGNEE_NONE,
          labelKey: "leads.columns.assignee",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: assigneeOptions,
          param: "assignee_user_id",
          cell: ({ row }) =>
            row.original.assignee_user_id
              ? `#${row.original.assignee_user_id}`
              : t("leads.form.unassigned"),
        }),
        createColumn<Lead>({
          accessorKey: "created_at",
          labelKey: "leads.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Lead>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.leads.detail(slug, row.original.uuid),
                  ),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Lead, unknown>[],
    [assigneeOptions, format, router, slug, t],
  );
  const columns = useMemo(
    () =>
      canWrite ? [createSelectColumnDef<Lead>(), ...baseColumns] : baseColumns,
    [baseColumns, canWrite],
  );

  // Column meta drives the params: facets (CSV), created (_from / _to).
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: LEAD_PAGE_SIZE,
    persistKey: LEADS_PERSIST_KEY,
  });
  const selectedAssignees = selectedValues(listState.columnFilters, "assignee");
  if (selectedAssignees.join(",") !== assigneeFilter.join(",")) {
    setAssigneeFilter(selectedAssignees);
  }

  const params = useMemo<LeadListQuery>(
    () => ({
      ...(listState.params as LeadListQuery),
      ...(followUp ? { follow_up: followUp } : {}),
    }),
    [followUp, listState.params],
  );

  const list = useQuery({
    queryKey: leadKeys.list(params),
    queryFn: () => leadsService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;
  const pageItems = list.data?.items;
  if (pageItems && pageItems !== items) setItems(pageItems);

  // Bulk "select all matching" and the export use the list filters,
  // search, follow-up tab and sort.
  const listQuery = useMemo(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(listState.filterParams)) {
      if (value) query[key] = String(value);
    }
    if (params.q) query.q = params.q;
    if (params.follow_up) query.follow_up = params.follow_up;
    if (params.sort) query.sort = params.sort;
    return query;
  }, [listState.filterParams, params.follow_up, params.q, params.sort]);

  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery: listQuery,
    total,
  });

  const changeFollowUp = (value: FollowUp) => {
    setFollowUp(value);
    listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
  };

  const title = t("leads.list.title");
  return (
    <EntityPage
      title={title}
      description={t("leads.list.description")}
      permission={permissions.leads.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("leads.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canWrite ? (
          <EntityCreateButton
            label={t("leads.list.new")}
            onClick={() => router.push(routes.tenant.leads.create(slug))}
          />
        ) : null
      }
    >
      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("leads.columns.follow_up")}
      >
        {FOLLOW_UPS.map((value) => (
          <Button
            key={value || "all"}
            type="button"
            role="tab"
            aria-selected={followUp === value}
            variant={followUp === value ? "default" : "outline"}
            size="sm"
            data-testid={`lead-tab-${value || "all"}`}
            onClick={() => changeFollowUp(value)}
          >
            {t(`leads.follow_up.${value || "all"}`)}
          </Button>
        ))}
      </div>
      {canWrite ? (
        <SelectionBanner
          selectedCount={bulkSelection.selectedCount}
          total={total}
          showSelectAll={bulkSelection.showSelectAllBanner}
          allMatchingSelected={bulkSelection.scope.mode === "all"}
          onSelectAllMatching={bulkSelection.selectAllMatching}
          onClearSelection={bulkSelection.clearSelection}
        />
      ) : null}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.leads.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("leads.list.empty_title")}
        emptyDescription={t("leads.list.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: LEADS_PERSIST_KEY,
          rowSelection: canWrite,
        }}
        renderGridItem={(lead) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <Link
                href={routes.tenant.leads.detail(slug, lead.uuid)}
                className="font-medium hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {leadName(lead)}
              </Link>
              <StatusChip
                label={t(`leads.status.${lead.status}`)}
                tone={leadStatusTone(lead.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {t(`leads.target_type.${lead.target_type}`)} ·{" "}
              {t(`leads.source.${lead.source}`)} ·{" "}
              {t(`leads.temperature.${lead.temperature}`)}
            </p>
            {lead.follow_up_date ? (
              <p className="text-muted-foreground text-xs">
                {t("leads.columns.follow_up")}:{" "}
                {format.dateTime(lead.follow_up_date)}
              </p>
            ) : null}
          </div>
        )}
        toolbarExtra={
          <>
            {canWrite ? (
              <BulkActionMenu
                resource="leads"
                actions={LEAD_BULK_ACTIONS}
                scope={bulkSelection.scope}
                selectedCount={bulkSelection.selectedCount}
                onComplete={() => {
                  bulkSelection.clearSelection();
                  void qc.invalidateQueries({ queryKey: leadKeys.all });
                }}
              />
            ) : null}
            <ExportMenu
              exportPath={LEADS_EXPORT_PATH}
              query={listQuery}
              formats={["xlsx", "csv", "pdf"]}
              jobsHref={routes.tenant.exports.root(slug)}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
