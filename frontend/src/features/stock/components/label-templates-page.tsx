"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Star, Tags, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { LabelTemplateFormDialog } from "@/features/stock/components/label-template-form";
import {
  emptyTemplateValues,
  TEMPLATE_KINDS,
  templateValues,
  toTemplateInput,
  type LabelTemplateValues,
} from "@/features/stock/lib/label-templates";
import {
  labelTemplateKeys,
  labelTemplatesService,
  type LabelTemplate,
} from "@/features/stock/services/label-templates.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const LABEL_TEMPLATES_PERSIST_KEY = "stock-label-templates-v1";

type FormState =
  { mode: "create" } | { mode: "edit"; target: LabelTemplate } | null;

/**
 * Stock settings › Label templates (TEC-764): the organization's unit and
 * location label templates (TEC-202) in a DataTable, a create / edit
 * dialog with a live preview, the default mark per kind and a confirmed
 * delete. Center and distributor (K12); stock.read lists, stock.write
 * changes.
 */
export function LabelTemplatesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const qc = useQueryClient();
  const { confirmDelete } = useDialogs();
  const fullWarehouse = org?.type === "center" || org?.type === "distributor";
  const canRead = fullWarehouse && can(Permission.StockRead);
  const canWrite = canRead && can(Permission.StockWrite);
  const [form, setForm] = useState<FormState>(null);

  const list = useQuery({
    queryKey: labelTemplateKeys.all,
    queryFn: () => labelTemplatesService.list(),
    enabled: canRead,
  });

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: labelTemplateKeys.all });

  const save = useMutation({
    mutationFn: ({
      f,
      v,
    }: {
      f: NonNullable<FormState>;
      v: LabelTemplateValues;
    }) =>
      f.mode === "create"
        ? labelTemplatesService.create(toTemplateInput(v))
        : labelTemplatesService.update(f.target.uuid, toTemplateInput(v)),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("stock.label_templates.saved"));
      setForm(null);
    },
  });

  const makeDefault = useMutation({
    mutationFn: (tpl: LabelTemplate) =>
      labelTemplatesService.update(
        tpl.uuid,
        toTemplateInput({ ...templateValues(tpl), is_default: true }),
      ),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("stock.label_templates.default_set"));
    },
    onError: () => appToast.error(t("stock.label_templates.errors.generic")),
  });

  const remove = useMutation({
    mutationFn: (uuid: string) => labelTemplatesService.remove(uuid),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("stock.label_templates.deleted"));
    },
    onError: () => appToast.error(t("stock.label_templates.errors.generic")),
  });

  const askDelete = async (tpl: LabelTemplate) => {
    const ok = await confirmDelete({
      title: t("stock.label_templates.delete.title", { name: tpl.name }),
      description: t(
        tpl.is_default
          ? "stock.label_templates.delete.description_default"
          : "stock.label_templates.delete.description",
      ),
      confirmLabel: t("common.delete"),
    });
    if (ok) remove.mutate(tpl.uuid);
  };

  const columns = useMemo(
    () =>
      [
        createColumn<LabelTemplate>({
          accessorKey: "name",
          labelKey: "stock.label_templates.fields.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="flex items-center gap-2 font-medium"
              data-testid="label-template-row"
              data-name={row.original.name}
            >
              {row.original.name}
              {row.original.is_default ? (
                <Badge variant="secondary" data-testid="label-template-default">
                  <Star className="size-3" />
                  {t("stock.label_templates.default")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<LabelTemplate>({
          accessorKey: "kind",
          labelKey: "stock.label_templates.fields.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: TEMPLATE_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `stock.label_templates.kind.${value}`,
          })),
          filterFn: "arrIncludesSome",
          gridSecondary: true,
          cell: ({ row }) =>
            t(`stock.label_templates.kind.${row.original.kind}`),
        }),
        createColumn<LabelTemplate>({
          accessorKey: "symbology",
          labelKey: "stock.label_templates.fields.symbology",
          enableSorting: true,
          cell: ({ row }) =>
            t(`stock.label_templates.symbology.${row.original.symbology}`),
        }),
        createColumn<LabelTemplate>({
          id: "size",
          accessorFn: (r) => Number(r.width_mm) * Number(r.height_mm),
          labelKey: "stock.label_templates.columns.size",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums" dir="ltr">
              {t("stock.label_templates.size_value", {
                width: row.original.width_mm,
                height: row.original.height_mm,
              })}
            </span>
          ),
        }),
        createColumn<LabelTemplate>({
          accessorKey: "columns",
          labelKey: "stock.label_templates.fields.columns",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.columns),
        }),
        createColumn<LabelTemplate>({
          id: "active",
          accessorFn: (r) => (r.active ? "true" : "false"),
          labelKey: "stock.label_templates.fields.active",
          enableSorting: true,
          filterVariant: "select",
          filterOptions: ["true", "false"].map((value) => ({
            value,
            label: value,
            labelKey: `stock.label_templates.active.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(
                `stock.label_templates.active.${row.original.active ? "true" : "false"}`,
              )}
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<LabelTemplate>({
          accessorKey: "updated_at",
          labelKey: "stock.label_templates.columns.updated",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.updated_at)}
            </span>
          ),
        }),
        createColumn<LabelTemplate>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const tpl = row.original;
            const actions: EntityRowAction[] = [
              {
                id: "edit",
                label: t("common.edit"),
                icon: Pencil,
                onSelect: () => setForm({ mode: "edit", target: tpl }),
              },
              {
                id: "default",
                label: t("stock.label_templates.make_default"),
                icon: Star,
                disabled:
                  tpl.is_default || !tpl.active || makeDefault.isPending,
                onSelect: () => makeDefault.mutate(tpl),
              },
              {
                id: "delete",
                label: t("common.delete"),
                icon: Trash2,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => void askDelete(tpl),
              },
            ];
            return canWrite ? <EntityRowActions actions={actions} /> : null;
          },
        }),
      ] as ColumnDef<LabelTemplate, unknown>[],
    // askDelete only reads stable hooks.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canWrite, format, makeDefault.isPending, t],
  );

  const title = t("stock.label_templates.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Tags className="size-6" />}
      description={t("stock.label_templates.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("stock.nav"), href: routes.tenant.stock.root(slug) },
        { label: title },
      ]}
      actions={
        canWrite ? (
          <Button
            type="button"
            onClick={() => setForm({ mode: "create" })}
            data-testid="label-template-new"
          >
            <Plus className="size-4" />
            {t("stock.label_templates.new")}
          </Button>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("stock.label_templates.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("stock.label_templates.empty_title")}
        emptyDescription={t("stock.label_templates.empty_description")}
        initialState={{ sorting: [{ id: "name", desc: false }] }}
        features={{
          persistKey: LABEL_TEMPLATES_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(tpl) => (
          <div className="space-y-1">
            <p className="flex items-center gap-2 font-medium">
              {tpl.name}
              {tpl.is_default ? (
                <Star
                  className="size-3.5"
                  aria-label={t("stock.label_templates.default")}
                />
              ) : null}
            </p>
            <p className="text-muted-foreground text-sm">
              {t(`stock.label_templates.kind.${tpl.kind}`)} ·{" "}
              {t("stock.label_templates.size_value", {
                width: tpl.width_mm,
                height: tpl.height_mm,
              })}
            </p>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      {form ? (
        <LabelTemplateFormDialog
          key={form.mode === "edit" ? form.target.uuid : "new"}
          mode={form.mode}
          initial={
            form.mode === "edit"
              ? templateValues(form.target)
              : emptyTemplateValues()
          }
          pending={save.isPending}
          onSubmit={(v) => save.mutateAsync({ f: form, v })}
          onClose={() => setForm(null)}
        />
      ) : null}
    </div>
  );
}
