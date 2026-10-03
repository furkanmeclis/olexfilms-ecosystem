"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowLeftRight, MoveRight, Plus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ALL,
  ListBody,
  Pager,
  StatusFilter,
} from "@/features/warehouse/components/list-controls";
import { NativeSelectField } from "@/features/warehouse/components/native-select-field";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  pageCount,
  warehouseErrorMessage,
} from "@/features/warehouse/lib/errors";
import {
  parseBarcodes,
  transferFormSchema,
  type TransferFormValues,
} from "@/features/warehouse/lib/forms";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import {
  TRANSFER_STATUSES,
  transferStatusTone,
} from "@/features/warehouse/lib/transfers";
import {
  warehouseKeys,
  warehouseService,
  type TransferListQuery,
  type Warehouse,
  type WarehouseMoveResult,
  type WarehouseTransferStatus,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const TRANSFER_PAGE_SIZE = 20;

/**
 * Warehouse > Transfers (TEC-205): a bin ↔ bin move inside a warehouse in
 * one step (scan units, scan the target bin) and the warehouse ↔ warehouse
 * transfer documents (draft → in transit → completed, or cancelled).
 */
export function TransfersPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [status, setStatus] = useState<WarehouseTransferStatus | typeof ALL>(
    ALL,
  );
  const [page, setPage] = useState(0);
  const [creating, setCreating] = useState(false);

  const query = useMemo<TransferListQuery>(
    () => ({
      ...(status === ALL ? {} : { status }),
      limit: TRANSFER_PAGE_SIZE,
      offset: page * TRANSFER_PAGE_SIZE,
    }),
    [status, page],
  );
  const list = useQuery({
    queryKey: warehouseKeys.transfers(query),
    queryFn: () => warehouseService.listTransfers(query),
    enabled: access.allowed,
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, TRANSFER_PAGE_SIZE);

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.transfers.title")}
      description={t("warehouse.transfers.description")}
      icon={<ArrowLeftRight className="size-6" />}
      actions={
        canWrite && !creating ? (
          <Button
            type="button"
            onClick={() => setCreating(true)}
            data-testid="transfer-new"
          >
            <Plus className="size-4" />
            {t("warehouse.transfers.new")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <NewTransferForm slug={slug} onCancel={() => setCreating(false)} />
      ) : null}

      {canWrite ? <BinMoveCard /> : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("warehouse.transfers.list_title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <StatusFilter
            value={status}
            statuses={TRANSFER_STATUSES}
            labelKey="warehouse.transfer_status"
            ariaLabel={t("warehouse.entries.status_filter")}
            onChange={(s) => {
              setStatus(s);
              setPage(0);
            }}
          />
          <ListBody
            isError={list.isError}
            isLoading={list.isLoading}
            isEmpty={rows.length === 0}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.transfers.empty_title")}
            emptyDescription={t("warehouse.transfers.empty_description")}
            emptyTestId="transfers-empty"
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm" data-testid="transfers-table">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.transfers.columns.no")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.transfers.columns.route")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.status")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.lines")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.created")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr
                      key={r.uuid}
                      className="hover:bg-accent/50 border-b last:border-0"
                      data-testid="transfer-row"
                    >
                      <td className="p-2">
                        <Link
                          href={routes.tenant.warehouse.transfer(slug, r.uuid)}
                          className="font-mono font-medium hover:underline"
                          dir="ltr"
                        >
                          {r.transfer_no}
                        </Link>
                        {r.note ? (
                          <div className="text-muted-foreground truncate text-xs">
                            {r.note}
                          </div>
                        ) : null}
                      </td>
                      <td className="p-2">
                        <span className="inline-flex items-center gap-1">
                          {r.from_warehouse.code}
                          <MoveRight className="size-3 rtl:rotate-180" />
                          {r.to_warehouse.code}
                        </span>
                      </td>
                      <td className="p-2">
                        <StatusChip
                          label={t(`warehouse.transfer_status.${r.status}`)}
                          tone={transferStatusTone(r.status)}
                        />
                      </td>
                      <td className="p-2">{format.number(r.line_count)}</td>
                      <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                        {format.dateTime(r.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </ListBody>
          <Pager
            page={page}
            pages={pages}
            total={total}
            busy={list.isFetching}
            hidden={rows.length === 0 && page === 0}
            onPage={setPage}
          />
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}

/** Draft warehouse transfer: source, target (different) and a note. */
export function NewTransferForm({
  slug,
  onCancel,
}: {
  slug: string;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active: Warehouse[] = (warehouses.data?.items ?? []).filter(
    (w) => w.active,
  );
  const options = active.map((w) => ({
    value: w.uuid,
    label: `${w.code} · ${w.name}`,
  }));

  const create = useMutation({
    mutationFn: (v: TransferFormValues) =>
      warehouseService.createTransfer({
        from_warehouse_uuid: v.from_warehouse_uuid,
        to_warehouse_uuid: v.to_warehouse_uuid,
        note: v.note || null,
      }),
    onSuccess: async (transfer) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "transfers"] });
      router.push(routes.tenant.warehouse.transfer(slug, transfer.uuid));
    },
  });

  return (
    <Card data-testid="transfer-form">
      <CardHeader>
        <CardTitle>{t("warehouse.transfers.new_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {warehouses.isSuccess && active.length < 2 ? (
          <p
            className="text-muted-foreground mb-4 text-sm"
            data-testid="transfer-need-two"
          >
            {t("warehouse.transfers.need_two_warehouses")}
          </p>
        ) : null}
        <AppForm<TransferFormValues>
          schema={transferFormSchema(t) as never}
          defaultValues={{
            from_warehouse_uuid: "",
            to_warehouse_uuid: "",
            note: "",
          }}
          onSubmit={async (v) => {
            setError(null);
            try {
              await create.mutateAsync(v);
            } catch (err) {
              setError(
                warehouseErrorMessage(err, t, t("warehouse.form.error")),
              );
            }
          }}
          className="space-y-4"
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <NativeSelectField
              name="from_warehouse_uuid"
              label={t("warehouse.fields.from_warehouse")}
              placeholder={t("warehouse.entries.pick_warehouse")}
              options={options}
              testId="transfer-from"
            />
            <NativeSelectField
              name="to_warehouse_uuid"
              label={t("warehouse.fields.to_warehouse")}
              placeholder={t("warehouse.entries.pick_warehouse")}
              options={options}
              testId="transfer-to"
            />
          </div>
          <AppInput
            name="note"
            label={t("warehouse.fields.note")}
            data-testid="transfer-note"
          />
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("warehouse.form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={create.isPending}
              data-testid="transfer-create"
            >
              {t("warehouse.transfers.create")}
            </Button>
          </div>
        </AppForm>
      </CardContent>
    </Card>
  );
}

/**
 * Bin ↔ bin move (POST /v1/warehouse/moves): collect unit barcodes, then
 * scan the target location QR; every unit gets one placement. Moving to
 * another warehouse is a warehouse transfer (the backend answers 400).
 */
export function BinMoveCard() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<WarehouseMoveResult | null>(null);
  const barcodes = parseBarcodes(text).map(unitBarcode);

  const move = useMutation({
    mutationFn: (locationCode: string) =>
      warehouseService.move({ barcodes, location_code: locationCode }),
    onSuccess: (res) => {
      setError(null);
      setResult(res);
      setText("");
      void qc.invalidateQueries({ queryKey: ["warehouse"] });
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.move.failed"))),
  });

  const onLocation = (code: string) => {
    if (scanKind(code) === "unit") {
      setText((prev) => (prev ? `${prev}\n${code}` : code));
      return;
    }
    if (barcodes.length === 0) {
      setError(t("warehouse.move.no_units"));
      return;
    }
    move.mutate(code);
  };

  return (
    <Card data-testid="bin-move">
      <CardHeader>
        <CardTitle>{t("warehouse.move.title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.move.description")}
        </p>
      </CardHeader>
      <CardContent className="grid gap-6 lg:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="move-units">{t("warehouse.move.units")}</Label>
          <Textarea
            id="move-units"
            value={text}
            rows={4}
            dir="ltr"
            className="font-mono"
            onChange={(e) => setText(e.target.value)}
            data-testid="move-units"
          />
          <p className="text-muted-foreground text-xs">
            {t("warehouse.move.units_hint", { count: barcodes.length })}
          </p>
        </div>
        <ScanInput
          id="move-location"
          autoFocus={false}
          label={t("warehouse.entry.scan_location")}
          hint={t("warehouse.move.location_hint")}
          busy={move.isPending}
          onScan={onLocation}
        />
        {error ? (
          <p
            role="alert"
            className="text-destructive text-sm lg:col-span-2"
            data-testid="move-error"
          >
            {error}
          </p>
        ) : null}
        {result ? (
          <p
            className="text-sm lg:col-span-2"
            role="status"
            data-testid="move-result"
          >
            {t("warehouse.move.done", {
              count: result.units.length,
              location: result.location.full_code ?? result.location.code,
            })}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
