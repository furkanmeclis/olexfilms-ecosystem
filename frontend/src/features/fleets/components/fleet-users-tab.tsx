"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Loader2, UserMinus, UserPlus } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import {
  FLEET_USER_STATUSES,
  fleetKeys,
  fleetsService,
  type FleetCard,
  type FleetUser,
} from "@/features/fleets/services/fleets.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const FLEET_USERS_PERSIST_KEY = "tenant-fleet-users-v1";

function InviteDialog({
  fleetUuid,
  open,
  onOpenChange,
}: {
  fleetUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [surname, setSurname] = useState("");

  const close = (next: boolean) => {
    if (!next) {
      setEmail("");
      setName("");
      setSurname("");
    }
    onOpenChange(next);
  };

  const invite = useMutation({
    mutationFn: () =>
      fleetsService.inviteUser(fleetUuid, {
        email: email.trim(),
        name: name.trim(),
        surname: surname.trim() || undefined,
      }),
    onSuccess: async () => {
      appToast.success(t("fleets.users.invited"));
      await qc.invalidateQueries({ queryKey: ["fleets", fleetUuid] });
      await qc.invalidateQueries({ queryKey: fleetKeys.card(fleetUuid) });
      close(false);
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!email.trim() || !name.trim()) return;
    invite.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("fleets.users.invite")}</DialogTitle>
          <DialogDescription>{t("fleets.users.invite_hint")}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-1.5">
            <Label htmlFor="fleet-invite-email">
              {t("fleets.fields.email")}
            </Label>
            <Input
              id="fleet-invite-email"
              type="email"
              required
              dir="ltr"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="fleet-invite-name">
                {t("fleets.fields.user_name")}
              </Label>
              <Input
                id="fleet-invite-name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="fleet-invite-surname">
                {t("fleets.fields.user_surname")}
              </Label>
              <Input
                id="fleet-invite-surname"
                value={surname}
                onChange={(e) => setSurname(e.target.value)}
              />
            </div>
          </div>
          {invite.isError && isApiError(invite.error) ? (
            <p className="text-destructive text-sm">{invite.error.message}</p>
          ) : null}
          <DialogFooter>
            <Button type="submit" disabled={invite.isPending}>
              {invite.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              {t("fleets.users.invite")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Fleet card > Users (TEC-477): GET /v1/fleets/{uuid}/users (name, e-mail,
 * status facet, primary user, last login); invite by e-mail, disable (not
 * the primary user, who owns the vehicles).
 */
export function FleetUsersTab({ fleet }: { fleet: FleetCard }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canManage = can(permissions.fleets.manage);
  const [inviteOpen, setInviteOpen] = useState(false);

  const disable = useMutation({
    mutationFn: (u: FleetUser) => fleetsService.disableUser(fleet.uuid, u.uuid),
    onSuccess: async () => {
      appToast.success(t("fleets.users.disabled"));
      await qc.invalidateQueries({ queryKey: ["fleets", fleet.uuid] });
    },
  });

  const columns = useMemo(
    () =>
      [
        createColumn<FleetUser>({
          id: "name",
          accessorFn: (row) => `${row.name} ${row.surname}`.trim(),
          labelKey: "fleets.users.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 font-medium">
              {`${row.original.name} ${row.original.surname}`.trim()}
              {row.original.is_primary ? (
                <Badge variant="secondary">{t("fleets.users.primary")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<FleetUser>({
          accessorKey: "email",
          labelKey: "fleets.fields.email",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <span dir="ltr">{row.original.email ?? "—"}</span>,
        }),
        createColumn<FleetUser>({
          accessorKey: "status",
          labelKey: "fleets.users.status",
          enableSorting: true,
          filterVariant: "faceted",
          param: "status",
          filterOptions: FLEET_USER_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.users.status_${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`fleets.users.status_${row.original.status}`)}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<FleetUser>({
          accessorKey: "last_login_at",
          labelKey: "fleets.users.last_login",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_login_at
              ? format.dateTime(row.original.last_login_at)
              : "—",
        }),
        createColumn<FleetUser>({
          accessorKey: "created_at",
          labelKey: "fleets.users.invited_at",
          enableSorting: true,
          cell: ({ row }) => format.date(row.original.created_at),
        }),
        createColumn<FleetUser>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "disable",
                  label: t("fleets.users.disable"),
                  icon: UserMinus,
                  variant: "destructive",
                  permission: permissions.fleets.manage,
                  disabled:
                    row.original.is_primary || row.original.status !== "active",
                  onSelect: () => disable.mutate(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FleetUser, unknown>[],
    [disable, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: FLEET_USERS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: fleetKeys.users(fleet.uuid, params),
    queryFn: () => fleetsService.listUsers(fleet.uuid, params),
  });

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("fleets.users.empty_title")}
        emptyDescription={t("fleets.users.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: FLEET_USERS_PERSIST_KEY }}
        toolbarExtra={
          <>
            {canManage ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setInviteOpen(true)}
              >
                <UserPlus className="size-4" />
                {t("fleets.users.invite")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {canManage ? (
        <InviteDialog
          fleetUuid={fleet.uuid}
          open={inviteOpen}
          onOpenChange={setInviteOpen}
        />
      ) : null}
    </>
  );
}
