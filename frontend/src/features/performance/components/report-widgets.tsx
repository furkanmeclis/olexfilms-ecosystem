"use client";

import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  rectSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { GripVertical, LayoutGrid, Plus, X } from "lucide-react";
import { useMemo, useState } from "react";

import { AppChart } from "@/components/charts";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { ErrorState } from "@/components/common/error-state";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  layoutInput,
  type EditableWidget,
} from "@/features/performance/lib/report-layout";
import {
  performanceKeys,
  performanceService,
  type ReportCatalogItem,
  type ReportEnvelope,
} from "@/features/performance/services/performance.service";
import { cn } from "@/lib/utils";
import { uuid } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Row = Record<string, unknown>;

/** Table body of a `table` report (client-side, the report is one page). */
function ReportTable({ report }: { report: ReportEnvelope }) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      report.columns.map((c) =>
        createColumn<Row>({
          id: c.key,
          accessorFn: (row) => row[c.key] as unknown,
          labelKey: c.label,
          label: c.label,
          cell: ({ row }) => String(row.original[c.key] ?? "—"),
        }),
      ) as ColumnDef<Row, unknown>[],
    [report.columns],
  );
  return (
    <EntityTable
      columns={columns}
      data={report.rows as Row[]}
      getRowId={(_row, index) => String(index)}
      manual={CLIENT_SIDE_MANUAL}
      features={{
        persistKey: `tenant-report-${report.report}-v1`,
        rowSelection: false,
        columnFilters: false,
        viewMode: false,
      }}
      emptyTitle={t("performance.empty")}
    />
  );
}

/** Renders one report by its kind (cards, series, distribution, table). */
function ReportBody({ report }: { report: ReportEnvelope }) {
  const { t, format } = useLocale();
  const series = report.series ?? [];
  if (report.kind === "table") return <ReportTable report={report} />;
  if (report.kind === "summary") {
    const points = series.flatMap((s) => s.points);
    return (
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {points.map((p) => (
          <div key={p.key} className="rounded-md border p-3">
            <dt className="text-muted-foreground text-xs">{p.label}</dt>
            <dd className="font-semibold tabular-nums">
              {format.number(p.value)}
            </dd>
          </div>
        ))}
      </dl>
    );
  }
  const first = series[0];
  if (!first) {
    return (
      <p className="text-muted-foreground text-sm">{t("performance.empty")}</p>
    );
  }
  if (report.kind === "timeseries") {
    const rows = first.points.map((p, i) => {
      const row: Row = { label: p.label };
      series.forEach((s) => {
        row[s.key] = s.points[i]?.value ?? null;
      });
      return row;
    });
    return (
      <AppChart
        type="line"
        data={rows}
        categoryKey="label"
        config={Object.fromEntries(
          series.map((s) => [s.key, { label: s.label }]),
        )}
        series={series.map((s) => s.key)}
        curved
        valueFormatter={(v) => format.number(v)}
        height={220}
        emptyTitle={t("performance.empty")}
      />
    );
  }
  const rows = first.points.map((p) => ({ label: p.label, value: p.value }));
  return (
    <AppChart
      type={report.kind === "distribution" ? "donut" : "bar-horizontal"}
      data={rows}
      categoryKey="label"
      config={{ value: { label: first.label } }}
      series={["value"]}
      valueFormatter={(v) => format.number(v)}
      height={220}
      emptyTitle={t("performance.empty")}
    />
  );
}

function WidgetCard({
  widget,
  item,
  editing,
  onRemove,
  onPeriod,
}: {
  widget: EditableWidget;
  item: ReportCatalogItem | undefined;
  editing: boolean;
  onRemove: () => void;
  onPeriod: (period: string) => void;
}) {
  const { t } = useLocale();
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: widget.id, disabled: !editing });
  const report = useQuery({
    queryKey: performanceKeys.report(widget.report, widget.period),
    queryFn: () =>
      performanceService.report(widget.report, { period: widget.period }),
    enabled: !editing,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };
  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(isDragging && "z-10 opacity-70")}
      data-testid="report-widget"
      data-report={widget.report}
    >
      <Card className="h-full">
        <CardHeader className="flex flex-row items-center gap-2">
          {editing ? (
            <button
              type="button"
              className="text-muted-foreground cursor-grab"
              aria-label={t("performance.widgets.drag")}
              {...attributes}
              {...listeners}
            >
              <GripVertical className="size-4" />
            </button>
          ) : null}
          <CardTitle className="flex-1 truncate">
            {item?.title ?? report.data?.title ?? widget.report}
          </CardTitle>
          {editing && item?.periodic ? (
            <Select
              value={widget.period ?? item.default_period ?? ""}
              onValueChange={onPeriod}
            >
              <SelectTrigger
                className="h-8 w-28"
                aria-label={t("performance.widgets.period")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {item.periods
                  .filter((p) => p !== "custom")
                  .map((p) => (
                    <SelectItem key={p} value={p}>
                      {t(`performance.widgets.periods.${p}`)}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          ) : null}
          {editing ? (
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("performance.widgets.remove")}
              onClick={onRemove}
            >
              <X className="size-4" />
            </Button>
          ) : null}
        </CardHeader>
        <CardContent>
          {editing ? (
            <p className="text-muted-foreground text-sm">
              {t("performance.widgets.editing_hint")}
            </p>
          ) : report.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void report.refetch()}
            />
          ) : report.isLoading || !report.data ? (
            <Skeleton className="h-40" />
          ) : (
            <ReportBody report={report.data} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Corporate dashboard: the legacy Admin / Dealer Filament widgets as
 * `/v1/reports/*` widgets. The layout (order, period) is the caller's own
 * (`/v1/reports/layout`); in edit mode widgets are dragged into order,
 * added from the catalog, removed or given another period, then saved.
 */
export function ReportWidgets() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const catalog = useQuery({
    queryKey: performanceKeys.reportCatalog,
    queryFn: () => performanceService.reportCatalog(),
    staleTime: 5 * 60 * 1000,
  });
  const layout = useQuery({
    queryKey: performanceKeys.reportLayout,
    queryFn: () => performanceService.reportLayout(),
  });
  const [draft, setDraft] = useState<EditableWidget[] | null>(null);
  const editing = draft !== null;
  const stored = useMemo<EditableWidget[]>(
    () =>
      (layout.data?.widgets ?? [])
        .slice()
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((w) => ({
          id: w.id,
          report: w.report,
          period: w.period ?? null,
          granularity: w.granularity ?? null,
        })),
    [layout.data],
  );
  const widgets = draft ?? stored;
  const byKey = useMemo(
    () => new Map((catalog.data?.items ?? []).map((i) => [i.key, i])),
    [catalog.data],
  );
  const addable = (catalog.data?.items ?? []).filter(
    (i) => !widgets.some((w) => w.report === i.key),
  );

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const onDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!draft || !over || active.id === over.id) return;
    const from = draft.findIndex((w) => w.id === active.id);
    const to = draft.findIndex((w) => w.id === over.id);
    if (from < 0 || to < 0) return;
    setDraft(arrayMove(draft, from, to));
  };

  const save = useMutation({
    mutationFn: (list: EditableWidget[]) =>
      performanceService.saveReportLayout(layoutInput(list)),
    onSuccess: (data) => {
      qc.setQueryData(performanceKeys.reportLayout, data);
      setDraft(null);
      appToast.success(t("performance.widgets.saved"));
    },
    onError: () => appToast.error(t("performance.widgets.failed")),
  });

  if (layout.isError || catalog.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => {
          void layout.refetch();
          void catalog.refetch();
        }}
      />
    );
  }

  return (
    <div className="space-y-4" data-testid="performance-widgets">
      <div className="flex flex-wrap items-center justify-end gap-2">
        {editing ? (
          <>
            {addable.length > 0 ? (
              <Select
                value=""
                onValueChange={(key) =>
                  setDraft([
                    ...(draft ?? []),
                    {
                      id: uuid(),
                      report: key,
                      period: byKey.get(key)?.default_period ?? null,
                      granularity: null,
                    },
                  ])
                }
              >
                <SelectTrigger
                  className="w-56"
                  aria-label={t("performance.widgets.add")}
                  data-testid="report-widget-add"
                >
                  <Plus className="size-4" />
                  <SelectValue placeholder={t("performance.widgets.add")} />
                </SelectTrigger>
                <SelectContent>
                  {addable.map((i) => (
                    <SelectItem key={i.key} value={i.key}>
                      {i.title}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            <Button variant="outline" onClick={() => setDraft(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              disabled={save.isPending}
              onClick={() => save.mutate(draft ?? [])}
              data-testid="report-widgets-save"
            >
              {t("common.save")}
            </Button>
          </>
        ) : (
          <Button
            variant="outline"
            onClick={() => setDraft(stored)}
            disabled={layout.isLoading}
            data-testid="report-widgets-edit"
          >
            <LayoutGrid className="size-4" />
            {t("performance.widgets.edit")}
          </Button>
        )}
      </div>
      {layout.isLoading ? (
        <Skeleton className="h-48" />
      ) : widgets.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("performance.widgets.empty")}
        </p>
      ) : (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={onDragEnd}
        >
          <SortableContext
            items={widgets.map((w) => w.id)}
            strategy={rectSortingStrategy}
          >
            <div className="grid gap-4 xl:grid-cols-2">
              {widgets.map((w) => (
                <WidgetCard
                  key={w.id}
                  widget={w}
                  item={byKey.get(w.report)}
                  editing={editing}
                  onRemove={() =>
                    setDraft((d) => (d ?? []).filter((x) => x.id !== w.id))
                  }
                  onPeriod={(period) =>
                    setDraft((d) =>
                      (d ?? []).map((x) =>
                        x.id === w.id ? { ...x, period } : x,
                      ),
                    )
                  }
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}
    </div>
  );
}
