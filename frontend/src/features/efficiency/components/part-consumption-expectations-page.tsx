"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, Trash2, Upload } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { ToolbarIconButton } from "@/components/tables/toolbar-icon-button";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import {
  applyExpectationEdit,
  expectationInput,
  expectationsCsv,
  metersValue,
} from "@/features/efficiency/lib/efficiency";
import {
  EXPECTATIONS_IMPORT_PATHS,
  efficiencyKeys,
  efficiencyService,
  type ExpectationInput,
  type Page,
  type PartConsumptionExpectation,
} from "@/features/efficiency/services/efficiency.service";
import { ImportWizard } from "@/features/io/components/import-wizard";
import {
  BODY_PARTS,
  WINDOW_PARTS,
  isKnownPart,
} from "@/features/services/lib/car-parts";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const EXPECTATIONS_PAGE_SIZE = 50;
export const EXPECTATIONS_PERSIST_KEY = "platform-part-expectations-v1";
export const EXPECTATIONS_IMPORT_RESOURCE =
  "platform.part_consumption_expectations";

const ALL_PARTS = [...BODY_PARTS, ...WINDOW_PARTS];
const EXPORT_PAGE = 200;

type Translate = ReturnType<typeof useLocale>["t"];

function partLabel(part: string, t: Translate) {
  return isKnownPart(part) ? t(`services.parts.names.${part}`) : part;
}

type TargetKind = "product" | "category";

function CreateDialog({
  open,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: ExpectationInput) => void;
}) {
  const { t } = useLocale();
  const [kind, setKind] = useState<TargetKind>("product");
  const [target, setTarget] = useState("");
  const [bodyType, setBodyType] = useState("");
  const [part, setPart] = useState("");
  const [meters, setMeters] = useState("");

  const loadProducts = useCallback(
    async (q: string): Promise<ComboboxOption[]> =>
      (await catalogService.listProducts({ q, limit: 20 })).items.map((p) => ({
        value: p.uuid,
        label: p.name,
        description: p.sku,
      })),
    [],
  );
  const loadCategories = useCallback(
    async (q: string): Promise<ComboboxOption[]> =>
      (await catalogService.listCategories({ q, limit: 20 })).items.map(
        (c) => ({ value: c.uuid, label: c.name }),
      ),
    [],
  );

  const metersNumber = Number(meters.replace(",", "."));
  const valid =
    Boolean(target) &&
    Boolean(part) &&
    Number.isFinite(metersNumber) &&
    metersNumber > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("efficiency.expectations.create")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>{t("efficiency.expectations.target")}</Label>
            <Select
              value={kind}
              onValueChange={(v) => {
                setKind(v as TargetKind);
                setTarget("");
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="product">
                  {t("efficiency.expectations.product")}
                </SelectItem>
                <SelectItem value="category">
                  {t("efficiency.expectations.category")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>
              {t(
                kind === "product"
                  ? "efficiency.expectations.product"
                  : "efficiency.expectations.category",
              )}
            </Label>
            <AsyncCombobox
              key={kind}
              value={target}
              onValueChange={setTarget}
              loadOptions={kind === "product" ? loadProducts : loadCategories}
              placeholder={t("efficiency.expectations.search")}
              searchPlaceholder={t("efficiency.expectations.search")}
              emptyText={t("efficiency.expectations.search_empty")}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="expectation-body-type">
                {t("efficiency.expectations.body_type")}
              </Label>
              <Input
                id="expectation-body-type"
                value={bodyType}
                placeholder={t("efficiency.body_type_all")}
                onChange={(e) => setBodyType(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="expectation-meters">
                {t("efficiency.expectations.expected_meters")}
              </Label>
              <Input
                id="expectation-meters"
                inputMode="decimal"
                value={meters}
                onChange={(e) => setMeters(e.target.value)}
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>{t("efficiency.expectations.part")}</Label>
            <Select value={part} onValueChange={setPart}>
              <SelectTrigger>
                <SelectValue
                  placeholder={t("efficiency.expectations.part_pick")}
                />
              </SelectTrigger>
              <SelectContent>
                {ALL_PARTS.map((p) => (
                  <SelectItem key={p} value={p}>
                    {partLabel(p, t)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!valid || pending}
            onClick={() =>
              onSubmit({
                product_uuid: kind === "product" ? target : null,
                category_uuid: kind === "category" ? target : null,
                body_type: bodyType.trim() || null,
                part_key: part,
                expected_meters: metersNumber.toFixed(2),
              })
            }
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function PartConsumptionExpectationsPage() {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [deleting, setDeleting] = useState<PartConsumptionExpectation | null>(
    null,
  );
  const [exporting, setExporting] = useState(false);

  const columns = useMemo(
    () =>
      [
        createColumn<PartConsumptionExpectation>({
          id: "product",
          accessorFn: (row) => row.product_name ?? row.category_name ?? "",
          labelKey: "efficiency.expectations.target",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div className="truncate font-medium">
                {row.original.product_name ?? row.original.category_name ?? "—"}
              </div>
              <div className="text-muted-foreground text-xs">
                {t(
                  row.original.product_uuid
                    ? "efficiency.expectations.product"
                    : "efficiency.expectations.category",
                )}
              </div>
            </div>
          ),
        }),
        createColumn<PartConsumptionExpectation>({
          accessorKey: "body_type",
          labelKey: "efficiency.expectations.body_type",
          enableSorting: true,
          filterVariant: "text",
          param: "body_type",
          editVariant: "text",
          cell: ({ row }) =>
            row.original.body_type || t("efficiency.body_type_all"),
        }),
        createColumn<PartConsumptionExpectation>({
          accessorKey: "part_key",
          labelKey: "efficiency.expectations.part",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ALL_PARTS.map((value) => ({
            value,
            label: value,
            labelKey: `services.parts.names.${value}`,
          })),
          param: "part_key",
          cell: ({ row }) => partLabel(row.original.part_key, t),
        }),
        createColumn<PartConsumptionExpectation>({
          accessorKey: "expected_meters",
          labelKey: "efficiency.expectations.expected_meters",
          enableSorting: true,
          editVariant: "number",
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(metersValue(row.original.expected_meters)),
        }),
        createColumn<PartConsumptionExpectation>({
          accessorKey: "source",
          labelKey: "efficiency.expectations.source",
          enableSorting: true,
          filterVariant: "select",
          filterOptions: (["manual", "network"] as const).map((value) => ({
            value,
            label: value,
            labelKey: `efficiency.expectations.sources.${value}`,
          })),
          param: "source",
          cell: ({ row }) => (
            <span data-testid="expectation-source">
              <StatusChip
                label={t(
                  `efficiency.expectations.sources.${row.original.source}`,
                )}
                tone={row.original.source === "network" ? "success" : "default"}
              />
            </span>
          ),
        }),
        createColumn<PartConsumptionExpectation>({
          accessorKey: "sample_size",
          labelKey: "efficiency.expectations.sample_size",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.sample_size),
        }),
        createColumn<PartConsumptionExpectation>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "delete",
                  label: t("common.delete"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => setDeleting(row.original),
                },
              ]}
            />
          ),
        }),
      ] satisfies ColumnDef<PartConsumptionExpectation, unknown>[],
    [format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-updated_at",
    initialPageSize: EXPECTATIONS_PAGE_SIZE,
    persistKey: EXPECTATIONS_PERSIST_KEY,
  });
  const listKey = efficiencyKeys.expectations(listState.params);
  const list = useQuery({
    queryKey: listKey,
    queryFn: () => efficiencyService.expectations(listState.params),
  });
  const invalidate = () =>
    void qc.invalidateQueries({
      queryKey: [...efficiencyKeys.all, "expectations"],
    });

  const update = useMutation({
    mutationFn: (row: PartConsumptionExpectation) =>
      efficiencyService.updateExpectation(row.uuid, expectationInput(row)),
    onSuccess: () => {
      appToast.success(t("efficiency.expectations.saved"));
      invalidate();
    },
    onError: () => {
      appToast.error(t("efficiency.expectations.save_failed"));
      invalidate();
    },
  });
  const create = useMutation({
    mutationFn: (input: ExpectationInput) =>
      efficiencyService.createExpectation(input),
    onSuccess: () => {
      setCreateOpen(false);
      appToast.success(t("efficiency.expectations.saved"));
      invalidate();
    },
    onError: () => appToast.error(t("efficiency.expectations.save_failed")),
  });
  const remove = useMutation({
    mutationFn: (row: PartConsumptionExpectation) =>
      efficiencyService.deleteExpectation(row.uuid),
    onSuccess: () => {
      setDeleting(null);
      appToast.success(t("efficiency.expectations.deleted"));
      invalidate();
    },
    onError: () => appToast.error(t("efficiency.expectations.save_failed")),
  });

  const exportCsv = async () => {
    setExporting(true);
    try {
      const rows: PartConsumptionExpectation[] = [];
      for (let offset = 0; ; offset += EXPORT_PAGE) {
        const page = await efficiencyService.expectations({
          ...listState.params,
          limit: EXPORT_PAGE,
          offset,
        });
        rows.push(...page.items);
        if (page.items.length === 0 || rows.length >= page.total) break;
      }
      const blob = new Blob([expectationsCsv(rows)], {
        type: "text/csv;charset=utf-8",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "part-consumption-expectations.csv";
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setExporting(false);
    }
  };

  const title = t("efficiency.expectations.title");
  return (
    <EntityPage
      title={title}
      description={t("efficiency.expectations.description")}
      permission={permissions.efficiency.expectationsManage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("efficiency.expectations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: title },
      ]}
      actions={
        <EntityCreateButton
          label={t("efficiency.expectations.create")}
          onClick={() => setCreateOpen(true)}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        rowCount={list.data?.total ?? 0}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        state={listState.tableState}
        features={{
          persistKey: EXPECTATIONS_PERSIST_KEY,
          rowSelection: false,
          inlineEdit: true,
          columnOrdering: true,
          columnPinning: true,
        }}
        onCellEdit={({ row, columnId, value }) => {
          const next = applyExpectationEdit(row, columnId, value);
          if (!next) {
            appToast.error(t("efficiency.expectations.invalid"));
            return;
          }
          // Optimistic: the edited row turns manual right away.
          qc.setQueryData<Page<PartConsumptionExpectation>>(listKey, (prev) =>
            prev
              ? {
                  ...prev,
                  items: prev.items.map((item) =>
                    item.uuid === next.uuid ? next : item,
                  ),
                }
              : prev,
          );
          update.mutate(next);
        }}
        toolbarExtra={
          <EntityToolbar>
            <ToolbarIconButton
              label={t("efficiency.expectations.export")}
              onClick={() => void exportCsv()}
              disabled={exporting}
            >
              <Download className="size-4" />
            </ToolbarIconButton>
            <ToolbarIconButton
              label={t("imports.menu_label")}
              onClick={() => setImportOpen(true)}
            >
              <Upload className="size-4" />
            </ToolbarIconButton>
          </EntityToolbar>
        }
        emptyTitle={t("efficiency.expectations.empty")}
      />
      {createOpen ? (
        <CreateDialog
          open
          pending={create.isPending}
          onOpenChange={setCreateOpen}
          onSubmit={(input) => create.mutate(input)}
        />
      ) : null}
      <ImportWizard
        resource={EXPECTATIONS_IMPORT_RESOURCE}
        paths={EXPECTATIONS_IMPORT_PATHS}
        open={importOpen}
        onOpenChange={setImportOpen}
        jobsHref={routes.platform.imports.root}
        onComplete={invalidate}
      />
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={
          deleting
            ? `${deleting.product_name ?? deleting.category_name ?? ""} · ${partLabel(deleting.part_key, t)}`
            : ""
        }
        softDelete={false}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
        onCancel={() => setDeleting(null)}
      />
    </EntityPage>
  );
}
