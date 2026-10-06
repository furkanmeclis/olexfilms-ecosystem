"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Star, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { ErrorState } from "@/components/common/error-state";
import {
  CLIENT_SIDE_MANUAL,
  EntityPage,
  EntityRowActions,
  EntityTable,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
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
import {
  useContractTemplates,
  useCreateContractTemplate,
  useDeleteContractTemplate,
  useSetDefaultContractTemplate,
} from "@/features/contracts/hooks/use-contract-templates";
import {
  CONTRACT_TEMPLATE_KINDS,
  type ContractTemplate,
  type ContractTemplateKind,
} from "@/features/contracts/services/contract-templates.service";
import { ApiError } from "@/lib/api/errors";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

export const CONTRACT_TEMPLATES_PERSIST_KEY = "platform-contract-templates-v1";

/**
 * Contract template list (TEC-290, TEC-370): client-side DataTable over the
 * full array with kind / active / default filters and row actions (edit,
 * make default, delete).
 */
export function ContractTemplatesPage() {
  const { t, format } = useLocale();
  const router = useRouter();
  const { confirmDelete } = useDialogs();
  const query = useContractTemplates();
  const setDefault = useSetDefaultContractTemplate();
  const removeTemplate = useDeleteContractTemplate();
  const remove = removeTemplate.mutate;
  const [pending, setPending] = useState<ContractTemplate | null>(null);
  const [creating, setCreating] = useState(false);
  const items = query.data?.items ?? [];
  const moduleOff =
    query.error instanceof ApiError && query.error.code === "FEATURE_DISABLED";

  const columns = useMemo<ColumnDef<ContractTemplate, unknown>[]>(
    () => [
      createColumn<ContractTemplate>({
        accessorKey: "name",
        labelKey: "contract_templates.fields.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className="flex flex-wrap items-center gap-2 font-medium"
            data-testid={`contract-template-${row.original.uuid}`}
          >
            {row.original.name}
            {row.original.is_default ? (
              <Badge variant="success" data-testid="default-badge">
                {t("contract_templates.default_badge")}
              </Badge>
            ) : null}
          </span>
        ),
      }) as ColumnDef<ContractTemplate, unknown>,
      createColumn<ContractTemplate>({
        accessorKey: "kind",
        labelKey: "contract_templates.fields.kind",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: CONTRACT_TEMPLATE_KINDS.map((value) => ({
          value,
          label: value,
          labelKey: `contract_templates.kinds.${value}`,
        })),
        cell: ({ row }) => t(`contract_templates.kinds.${row.original.kind}`),
      }) as ColumnDef<ContractTemplate, unknown>,
      createColumn<ContractTemplate>({
        accessorKey: "is_active",
        labelKey: "contract_templates.fields.status",
        enableSorting: true,
        filterVariant: "boolean",
        cell: ({ row }) => (
          <Badge variant={row.original.is_active ? "secondary" : "outline"}>
            {row.original.is_active
              ? t("contract_templates.status.active")
              : t("contract_templates.status.inactive")}
          </Badge>
        ),
      }) as ColumnDef<ContractTemplate, unknown>,
      createColumn<ContractTemplate>({
        accessorKey: "is_default",
        labelKey: "contract_templates.fields.is_default",
        enableSorting: true,
        filterVariant: "boolean",
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.is_default ? t("common.yes") : t("common.no"),
      }) as ColumnDef<ContractTemplate, unknown>,
      createColumn<ContractTemplate>({
        accessorKey: "updated_at",
        labelKey: "contract_templates.fields.updated_at",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {format.dateTime(row.original.updated_at)}
          </span>
        ),
      }) as ColumnDef<ContractTemplate, unknown>,
      createColumn<ContractTemplate>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const template = row.original;
          const actions: EntityRowAction[] = [
            {
              id: "edit",
              label: t("contract_templates.actions.edit"),
              icon: Pencil,
              onSelect: () =>
                router.push(
                  routes.platform.contractTemplates.edit(template.uuid),
                ),
            },
          ];
          if (!template.is_default && template.is_active) {
            actions.push({
              id: "make_default",
              label: t("contract_templates.actions.make_default"),
              icon: Star,
              onSelect: () => setPending(template),
            });
          }
          actions.push({
            id: "delete",
            label: t("contract_templates.actions.delete"),
            icon: Trash2,
            variant: "destructive",
            onSelect: () => {
              void (async () => {
                const ok = await confirmDelete({
                  title: t("contract_templates.delete_confirm.title"),
                  description: t(
                    "contract_templates.delete_confirm.description",
                    { name: template.name },
                  ),
                });
                if (ok) remove(template.uuid);
              })();
            },
          });
          return <EntityRowActions actions={actions} />;
        },
      }) as ColumnDef<ContractTemplate, unknown>,
    ],
    [confirmDelete, format, remove, router, t],
  );

  return (
    <EntityPage
      title={t("contract_templates.title")}
      description={t("contract_templates.description")}
      permission={permissions.contractTemplates.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("contract_templates.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("contract_templates.title") },
      ]}
      actions={
        <Button onClick={() => setCreating(true)} disabled={moduleOff}>
          {t("contract_templates.actions.new")}
        </Button>
      }
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={
          moduleOff
            ? t("contract_templates.module_disabled")
            : t("contract_templates.load_failed")
        }
        onRetry={moduleOff ? undefined : () => void query.refetch()}
        emptyTitle={t("contract_templates.empty")}
        emptyDescription=""
        onRowClick={(row) =>
          router.push(routes.platform.contractTemplates.edit(row.uuid))
        }
        initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
        features={{
          persistKey: CONTRACT_TEMPLATES_PERSIST_KEY,
          rowSelection: false,
        }}
      />

      <ConfirmDialog
        open={pending !== null}
        title={t("contract_templates.default_confirm.title")}
        description={
          pending
            ? t("contract_templates.default_confirm.description", {
                name: pending.name,
                kind: t(`contract_templates.kinds.${pending.kind}`),
              })
            : undefined
        }
        confirmLabel={t("contract_templates.actions.make_default")}
        isPending={setDefault.isPending}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (!pending) return;
          setDefault.mutate(pending.uuid, {
            onSettled: () => setPending(null),
          });
        }}
      />

      {creating ? (
        <CreateTemplateDialog onClose={() => setCreating(false)} />
      ) : null}
    </EntityPage>
  );
}

function CreateTemplateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateContractTemplate();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<ContractTemplateKind>("vehicle_intake");

  const submit = async () => {
    const row = await create.mutateAsync({
      name: name.trim(),
      kind,
      is_active: true,
    });
    onClose();
    router.push(routes.platform.contractTemplates.edit(row.uuid));
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !create.isPending) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("contract_templates.create.title")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="contract-template-name">
              {t("contract_templates.fields.name")}
            </Label>
            <Input
              id="contract-template-name"
              value={name}
              maxLength={150}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>{t("contract_templates.fields.kind")}</Label>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as ContractTemplateKind)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CONTRACT_TEMPLATE_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`contract_templates.kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={create.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => void submit().catch(() => undefined)}
            disabled={create.isPending || !name.trim()}
          >
            {t("contract_templates.actions.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
