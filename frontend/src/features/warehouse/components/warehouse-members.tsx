"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Star, Trash2, UserPlus, Users } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type MemberListQuery,
  type Warehouse,
  type WarehouseMember,
  type WarehouseMemberUser,
} from "@/features/warehouse/services/warehouse.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const MEMBERS_PAGE_SIZE = 20;
export const MEMBERS_PERSIST_KEY = "tenant-warehouse-members-v1";

export function memberName(u: WarehouseMemberUser): string {
  return `${u.name} ${u.surname}`.trim();
}

/**
 * Members of a warehouse (TEC-769). Without any membership a user works in
 * every warehouse of the organization; a member of at least one warehouse
 * writes only in its member warehouses. Server DataTable: sort (name,
 * email, added), search, remove row action; members.write adds / removes.
 */
export function WarehouseMembersCard({
  warehouse,
  canManage,
}: {
  warehouse: Warehouse;
  canManage: boolean;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const { confirmDelete } = useDialogs();
  const [adding, setAdding] = useState(false);

  const remove = useMutation({
    mutationFn: (m: WarehouseMember) =>
      warehouseService.removeMember(warehouse.uuid, m.user.uuid),
    onSuccess: async () => {
      appToast.success(t("warehouse.members.removed"));
      await qc.invalidateQueries({ queryKey: ["warehouse", "members"] });
      await qc.invalidateQueries({ queryKey: warehouseKeys.mine });
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const columns = useMemo(() => {
    const askRemove = async (m: WarehouseMember) => {
      const ok = await confirmDelete({
        title: t("warehouse.members.remove_title", {
          name: memberName(m.user),
        }),
        description: t("warehouse.members.remove_description"),
        confirmLabel: t("warehouse.members.remove"),
      });
      if (ok) remove.mutate(m);
    };
    const cols: ColumnDef<WarehouseMember, unknown>[] = [
      createColumn<WarehouseMember>({
        id: "name",
        accessorFn: (m) => memberName(m.user),
        labelKey: "warehouse.members.columns.name",
        enableSorting: true,
        sortParam: "name",
        gridPrimary: true,
        cell: ({ row }) => (
          <span data-testid="member-row" data-uuid={row.original.user.uuid}>
            {memberName(row.original.user)}
          </span>
        ),
      }),
      createColumn<WarehouseMember>({
        id: "email",
        accessorFn: (m) => m.user.email ?? "",
        labelKey: "warehouse.members.columns.email",
        enableSorting: true,
        sortParam: "email",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm" dir="ltr">
            {row.original.user.email ?? "—"}
          </span>
        ),
      }),
      createColumn<WarehouseMember>({
        accessorKey: "is_default",
        labelKey: "warehouse.members.columns.default",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.is_default ? (
            <Badge variant="secondary" className="gap-1">
              <Star className="size-3" aria-hidden />
              {t("warehouse.members.default")}
            </Badge>
          ) : null,
      }),
      createColumn<WarehouseMember>({
        accessorKey: "created_at",
        labelKey: "warehouse.members.columns.added",
        enableSorting: true,
        sortParam: "created_at",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
    ];
    if (canManage) {
      cols.push(
        createColumn<WarehouseMember>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const items: EntityRowAction[] = [
              {
                id: "remove",
                label: t("warehouse.members.remove"),
                icon: Trash2,
                variant: "destructive",
                onSelect: () => void askRemove(row.original),
              },
            ];
            return <EntityRowActions actions={items} />;
          },
        }),
      );
    }
    return cols;
  }, [canManage, confirmDelete, format, remove, t]);

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: MEMBERS_PAGE_SIZE,
    persistKey: MEMBERS_PERSIST_KEY,
  });
  const query: MemberListQuery = listState.params;
  const list = useQuery({
    queryKey: warehouseKeys.members(warehouse.uuid, query),
    queryFn: () => warehouseService.listMembers(warehouse.uuid, query),
  });

  return (
    <Card data-testid="warehouse-members">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Users className="size-4" aria-hidden />
          {t("warehouse.members.title", { code: warehouse.code })}
        </CardTitle>
        {canManage ? (
          <Button
            size="sm"
            onClick={() => setAdding(true)}
            data-testid="member-add"
          >
            <UserPlus className="size-4" aria-hidden />
            {t("warehouse.members.add")}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-muted-foreground text-sm">
          {t("warehouse.members.description")}
        </p>
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t("warehouse.members.empty")}
          emptyDescription={t("warehouse.members.empty_description")}
          rowCount={list.data?.total ?? 0}
          state={listState.tableState}
          features={{
            persistKey: MEMBERS_PERSIST_KEY,
            rowSelection: false,
            viewMode: true,
          }}
          renderGridItem={(m) => (
            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{memberName(m.user)}</span>
                {m.is_default ? (
                  <Badge variant="secondary">
                    {t("warehouse.members.default")}
                  </Badge>
                ) : null}
              </div>
              <p className="text-muted-foreground text-xs" dir="ltr">
                {m.user.email ?? "—"}
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
      </CardContent>
      {adding ? (
        <AddMemberDialog
          warehouse={warehouse}
          onClose={() => setAdding(false)}
        />
      ) : null}
    </Card>
  );
}

export function AddMemberDialog({
  warehouse,
  onClose,
}: {
  warehouse: Warehouse;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [userUuid, setUserUuid] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const add = useMutation({
    mutationFn: () =>
      warehouseService.addMember(warehouse.uuid, {
        user_uuid: userUuid,
        is_default: isDefault,
      }),
    onSuccess: async () => {
      appToast.success(t("warehouse.members.added"));
      await qc.invalidateQueries({ queryKey: ["warehouse", "members"] });
      await qc.invalidateQueries({ queryKey: warehouseKeys.mine });
      onClose();
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const loadOptions = async (q: string): Promise<ComboboxOption[]> => {
    const res = await warehouseService.memberCandidates(q);
    return res.items.map((u) => ({
      value: u.uuid,
      label: memberName(u),
      description: u.email ?? undefined,
    }));
  };

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent data-testid="member-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("warehouse.members.add_title", { code: warehouse.code })}
          </DialogTitle>
          <DialogDescription>
            {t("warehouse.members.add_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="member-user">{t("warehouse.members.user")}</Label>
            <AsyncCombobox
              id="member-user"
              value={userUuid}
              onValueChange={setUserUuid}
              loadOptions={loadOptions}
              placeholder={t("warehouse.members.pick_user")}
              searchPlaceholder={t("warehouse.members.search_user")}
              emptyText={t("warehouse.members.no_candidates")}
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="member-default"
              checked={isDefault}
              onCheckedChange={(v) => setIsDefault(v === true)}
              data-testid="member-default"
            />
            <Label htmlFor="member-default" className="font-normal">
              {t("warehouse.members.make_default")}
            </Label>
          </div>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("warehouse.form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!userUuid || add.isPending}
            onClick={() => {
              setError(null);
              add.mutate();
            }}
            data-testid="member-save"
          >
            {t("warehouse.members.add")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
