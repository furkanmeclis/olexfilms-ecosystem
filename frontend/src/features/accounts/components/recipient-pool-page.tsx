"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus, Store, UserCheck } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AccountFields,
  accountBody,
  emptyAccount,
  type AccountValues,
} from "@/features/accounts/components/account-fields";
import { NewRecipientDialog } from "@/features/accounts/components/new-recipient-dialog";
import {
  RECIPIENT_KINDS,
  pendingRecipientKeys,
  pendingRecipientsService,
  type PendingRecipient,
  type PendingRecipientState,
} from "@/features/accounts/services/pending-recipients.service";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const RECIPIENT_POOL_PERSIST_KEY = "tenant-recipient-pool-v1";

const STATE_TONE = {
  pending: "warning",
  retail: "default",
  converted: "success",
} as const;

/**
 * Turns a recipient into a dealer: the owner (optional) is the person who
 * signs in on the phone; the recipient's phone is the default.
 */
function ConvertDialog({
  recipient,
  onOpenChange,
}: {
  recipient: PendingRecipient | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [owner, setOwner] = useState<AccountValues>(() => ({
    ...emptyAccount,
    phone: recipient?.phone ?? "",
  }));
  const withOwner = owner.name.trim() !== "" && owner.phone.trim() !== "";
  const convert = useMutation({
    mutationFn: () =>
      pendingRecipientsService.convert(
        recipient?.uuid ?? "",
        withOwner ? { owner: accountBody(owner) } : {},
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pendingRecipientKeys.all });
      void qc.invalidateQueries({ queryKey: ["tenant", "organizations"] });
      toast.success(t("organizations.recipients.converted"));
      onOpenChange(false);
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("organizations.recipients.save_failed")),
      ),
  });
  return (
    <Dialog open={recipient !== null} onOpenChange={onOpenChange}>
      <DialogContent data-testid="recipient-convert-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("organizations.recipients.convert_title", {
              name: recipient?.name ?? "",
            })}
          </DialogTitle>
          <DialogDescription>
            {t("organizations.recipients.convert_description")}
          </DialogDescription>
        </DialogHeader>
        <AccountFields
          idPrefix="recipient-owner"
          value={owner}
          onChange={setOwner}
          touched={false}
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={convert.isPending}
            data-testid="recipient-convert-save"
            onClick={() => convert.mutate()}
          >
            {t("organizations.recipients.convert")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * TEC-1062: the center's waiting pool. Recipients opened from the order
 * form without a dealer record wait here (tab "pool"); the center turns
 * one into a dealer or keeps it as a retail / delivery point.
 */
export function RecipientPoolPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canWrite = can(Permission.OrganizationsWrite);
  const canCreate = canWrite || can(Permission.OrdersWrite);
  const [tab, setTab] = useState<PendingRecipientState>("pending");
  const [converting, setConverting] = useState<PendingRecipient | null>(null);
  const [convertKey, setConvertKey] = useState(0);
  const [creating, setCreating] = useState(false);

  const keepRetail = useMutation({
    mutationFn: (r: PendingRecipient) =>
      pendingRecipientsService.keepRetail(r.uuid),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pendingRecipientKeys.all });
      toast.success(t("organizations.recipients.kept_retail"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("organizations.recipients.save_failed")),
      ),
  });
  const keep = keepRetail.mutate;

  const columns = useMemo(
    () =>
      [
        createColumn<PendingRecipient>({
          accessorKey: "name",
          labelKey: "organizations.recipients.columns.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium" data-testid="recipient-row">
              {row.original.name}
            </span>
          ),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "phone",
          labelKey: "organizations.recipients.columns.phone",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span dir="ltr" className="tabular-nums">
              {row.original.phone || "—"}
            </span>
          ),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "kind",
          labelKey: "organizations.recipients.columns.kind",
          enableSorting: true,
          filterVariant: "faceted",
          param: "kind",
          filterOptions: RECIPIENT_KINDS.map((value) => ({
            value,
            labelKey: `organizations.recipients.kinds.${value}`,
            label: value,
          })),
          cell: ({ row }) =>
            t(`organizations.recipients.kinds.${row.original.kind}`),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "state",
          labelKey: "organizations.recipients.columns.state",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`organizations.recipients.states.${row.original.state}`)}
              tone={STATE_TONE[row.original.state]}
            />
          ),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "city",
          labelKey: "organizations.recipients.columns.city",
          enableSorting: true,
          cell: ({ row }) =>
            [row.original.city, row.original.district]
              .filter(Boolean)
              .join(" / ") || "—",
        }),
        createColumn<PendingRecipient>({
          accessorKey: "address",
          labelKey: "organizations.recipients.columns.address",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.address || "—",
        }),
        createColumn<PendingRecipient>({
          accessorKey: "tax_no",
          labelKey: "organizations.recipients.columns.tax_no",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span dir="ltr">{row.original.tax_no ?? "—"}</span>
          ),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "orders_count",
          labelKey: "organizations.recipients.columns.orders",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.orders_count}</span>
          ),
        }),
        createColumn<PendingRecipient>({
          accessorKey: "created_at",
          labelKey: "organizations.recipients.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<PendingRecipient>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const r = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "convert",
                    label: t("organizations.recipients.convert"),
                    icon: UserCheck,
                    disabled: !canWrite || r.state === "converted",
                    onSelect: () => {
                      setConvertKey((k) => k + 1);
                      setConverting(r);
                    },
                  },
                  {
                    id: "keep-retail",
                    label: t("organizations.recipients.keep_retail"),
                    icon: Store,
                    disabled: !canWrite || r.state !== "pending",
                    onSelect: () => keep(r),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<PendingRecipient, unknown>[],
    [canWrite, format, keep, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: RECIPIENT_POOL_PERSIST_KEY,
  });
  const params = useMemo(
    () => ({ ...listState.params, state: tab }),
    [listState.params, tab],
  );
  const list = useQuery({
    queryKey: pendingRecipientKeys.list(params),
    queryFn: () => pendingRecipientsService.list(params),
  });

  return (
    <EntityPage
      title={t("organizations.recipients.pool_title")}
      description={t("organizations.recipients.pool_description")}
      permission={[Permission.OrganizationsRead, Permission.OrdersWrite]}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.accounts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.nav_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("organizations.recipients.pool_title") },
      ]}
    >
      <div className="space-y-4" data-testid="recipient-pool-page">
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as PendingRecipientState)}
        >
          <TabsList>
            <TabsTrigger value="pending" data-testid="recipient-tab-pending">
              {t("organizations.recipients.tabs.pending")}
            </TabsTrigger>
            <TabsTrigger value="retail" data-testid="recipient-tab-retail">
              {t("organizations.recipients.tabs.retail")}
            </TabsTrigger>
            <TabsTrigger
              value="converted"
              data-testid="recipient-tab-converted"
            >
              {t("organizations.recipients.tabs.converted")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t("organizations.recipients.empty")}
          rowCount={list.data?.total ?? 0}
          state={listState.tableState}
          features={{ persistKey: RECIPIENT_POOL_PERSIST_KEY }}
          toolbarExtra={
            <>
              {canCreate ? (
                <Button
                  type="button"
                  size="sm"
                  data-testid="recipient-create"
                  onClick={() => setCreating(true)}
                >
                  <Plus className="size-4" />
                  {t("organizations.recipients.new")}
                </Button>
              ) : null}
              <EntityToolbar
                onRefresh={() => void list.refetch()}
                refreshDisabled={list.isFetching}
              />
            </>
          }
        />
      </div>
      <NewRecipientDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => setTab("pending")}
      />
      <ConvertDialog
        key={convertKey}
        recipient={converting}
        onOpenChange={(open) => {
          if (!open) setConverting(null);
        }}
      />
    </EntityPage>
  );
}
