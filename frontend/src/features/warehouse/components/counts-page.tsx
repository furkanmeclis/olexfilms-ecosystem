"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ClipboardCheck, Plus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import {
  ALL,
  ListBody,
  Pager,
  StatusFilter,
} from "@/features/warehouse/components/list-controls";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  COUNT_METHODS,
  COUNT_STATUSES,
  COUNT_VISIBILITIES,
  countBody,
  countFormErrors,
  countStatusTone,
  EMPTY_COUNT_FORM,
  scopesFor,
  type CountFormState,
} from "@/features/warehouse/lib/counts";
import {
  pageCount,
  warehouseErrorMessage,
} from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type CountListQuery,
  type StockCount,
  type StockCountStatus,
} from "@/features/warehouse/services/warehouse.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

export const COUNT_PAGE_SIZE = 20;

/** Scope summary of a count: warehouse, room, location or product. */
export function countScopeText(
  c: StockCount,
  t: (k: string) => string,
): string {
  switch (c.scope_type) {
    case "room":
      return c.scope_room ? `${c.scope_room.code} · ${c.scope_room.name}` : "";
    case "location":
      return c.scope_location?.full_code ?? "";
    case "product":
      return c.scope_product
        ? `${c.scope_product.sku} · ${c.scope_product.name}`
        : "";
    default:
      return t("warehouse.count_scope.warehouse");
  }
}

/**
 * Warehouse > Stock counts (TEC-206): the counts of the organization and a
 * new draft (warehouse, method, blind / guided, scope).
 */
export function CountsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [status, setStatus] = useState<StockCountStatus | typeof ALL>(ALL);
  const [page, setPage] = useState(0);
  const [creating, setCreating] = useState(false);

  const query = useMemo<CountListQuery>(
    () => ({
      ...(status === ALL ? {} : { status }),
      limit: COUNT_PAGE_SIZE,
      offset: page * COUNT_PAGE_SIZE,
    }),
    [status, page],
  );
  const list = useQuery({
    queryKey: warehouseKeys.counts(query),
    queryFn: () => warehouseService.listCounts(query),
    enabled: access.allowed,
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, COUNT_PAGE_SIZE);

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.counts.title")}
      description={t("warehouse.counts.description")}
      icon={<ClipboardCheck className="size-6" />}
      actions={
        canWrite && !creating ? (
          <Button
            type="button"
            onClick={() => setCreating(true)}
            data-testid="count-new"
          >
            <Plus className="size-4" />
            {t("warehouse.counts.new")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <NewCountForm slug={slug} onCancel={() => setCreating(false)} />
      ) : null}

      <Card>
        <CardContent className="space-y-4 pt-6">
          <StatusFilter
            value={status}
            statuses={COUNT_STATUSES}
            labelKey="warehouse.count_status"
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
            emptyTitle={t("warehouse.counts.empty_title")}
            emptyDescription={t("warehouse.counts.empty_description")}
            emptyTestId="counts-empty"
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm" data-testid="counts-table">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.warehouse")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.fields.method")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.fields.scope")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.status")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.created")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((c) => (
                    <tr
                      key={c.uuid}
                      className="hover:bg-accent/50 border-b last:border-0"
                      data-testid="count-row"
                    >
                      <td className="p-2">
                        <Link
                          href={routes.tenant.warehouse.count(slug, c.uuid)}
                          className="font-medium hover:underline"
                        >
                          {c.warehouse.code} · {c.warehouse.name}
                        </Link>
                      </td>
                      <td className="p-2">
                        {t(`warehouse.count_method.${c.method}`)}
                        <div className="text-muted-foreground text-xs">
                          {t(`warehouse.count_visibility.${c.visibility}`)}
                        </div>
                      </td>
                      <td className="p-2">{countScopeText(c, t)}</td>
                      <td className="p-2">
                        <StatusChip
                          label={t(`warehouse.count_status.${c.status}`)}
                          tone={countStatusTone(c.status)}
                        />
                      </td>
                      <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                        {format.dateTime(c.created_at)}
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

function FieldError({ id, text }: { id: string; text?: string }) {
  return text ? (
    <p id={id} role="alert" className="text-destructive text-xs">
      {text}
    </p>
  ) : null;
}

/**
 * New stock count: warehouse, one of the four methods, blind or guided and
 * a scope (warehouse, room, location subtree, product). initial_placement
 * has no product scope; its start needs an approval on the detail page.
 */
export function NewCountForm({
  slug,
  onCancel,
}: {
  slug: string;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [v, setV] = useState<CountFormState>(EMPTY_COUNT_FORM);
  const [errors, setErrors] = useState<
    Partial<Record<keyof CountFormState, string>>
  >({});
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const productQuery = useDebounce(q, 300);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active = (warehouses.data?.items ?? []).filter((w) => w.active);
  const products = useQuery({
    queryKey: ["warehouse", "catalog-products", productQuery],
    queryFn: () =>
      catalogService.listProducts({
        q: productQuery || undefined,
        active: true,
        limit: 50,
      }),
    enabled: v.scope_type === "product",
  });

  const set = <K extends keyof CountFormState>(
    key: K,
    value: CountFormState[K],
  ) => setV((prev) => ({ ...prev, [key]: value }));

  const create = useMutation({
    mutationFn: () => warehouseService.createCount(countBody(v)),
    onSuccess: async (count) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
      router.push(routes.tenant.warehouse.count(slug, count.uuid));
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next = countFormErrors(v, t);
    setErrors(next);
    setError(null);
    if (Object.keys(next).length === 0) create.mutate();
  };

  const scopes = scopesFor(v.method);

  return (
    <Card data-testid="count-form">
      <CardHeader>
        <CardTitle>{t("warehouse.counts.new_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-1.5">
              <Label htmlFor="count-warehouse">
                {t("warehouse.fields.warehouse")}
              </Label>
              <select
                id="count-warehouse"
                className={nativeSelectClass}
                value={v.warehouse_uuid}
                aria-invalid={Boolean(errors.warehouse_uuid)}
                data-testid="count-warehouse"
                onChange={(e) =>
                  setV((prev) => ({
                    ...prev,
                    warehouse_uuid: e.target.value,
                    room_uuid: "",
                    location_uuid: "",
                  }))
                }
              >
                <option value="">
                  {t("warehouse.entries.pick_warehouse")}
                </option>
                {active.map((w) => (
                  <option key={w.uuid} value={w.uuid}>
                    {w.code} · {w.name}
                  </option>
                ))}
              </select>
              <FieldError
                id="count-warehouse-error"
                text={errors.warehouse_uuid}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="count-method">
                {t("warehouse.fields.method")}
              </Label>
              <select
                id="count-method"
                className={nativeSelectClass}
                value={v.method}
                data-testid="count-method"
                onChange={(e) => {
                  const method = e.target.value as CountFormState["method"];
                  setV((prev) => ({
                    ...prev,
                    method,
                    scope_type: scopesFor(method).includes(prev.scope_type)
                      ? prev.scope_type
                      : "warehouse",
                  }));
                }}
              >
                {COUNT_METHODS.map((m) => (
                  <option key={m} value={m}>
                    {t(`warehouse.count_method.${m}`)}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                {t(`warehouse.count_method_hint.${v.method}`)}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="count-visibility">
                {t("warehouse.fields.visibility")}
              </Label>
              <select
                id="count-visibility"
                className={nativeSelectClass}
                value={v.visibility}
                data-testid="count-visibility"
                onChange={(e) =>
                  set(
                    "visibility",
                    e.target.value as CountFormState["visibility"],
                  )
                }
              >
                {COUNT_VISIBILITIES.map((m) => (
                  <option key={m} value={m}>
                    {t(`warehouse.count_visibility.${m}`)}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                {t(`warehouse.count_visibility_hint.${v.visibility}`)}
              </p>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="count-scope">{t("warehouse.fields.scope")}</Label>
            <select
              id="count-scope"
              className={nativeSelectClass}
              value={v.scope_type}
              data-testid="count-scope"
              onChange={(e) =>
                setV((prev) => ({
                  ...prev,
                  scope_type: e.target.value as CountFormState["scope_type"],
                  room_uuid: "",
                  location_uuid: "",
                  product_uuid: "",
                }))
              }
            >
              {scopes.map((s) => (
                <option key={s} value={s}>
                  {t(`warehouse.count_scope.${s}`)}
                </option>
              ))}
            </select>
            <FieldError id="count-scope-error" text={errors.scope_type} />
          </div>

          {v.scope_type === "room" || v.scope_type === "location" ? (
            <div className="space-y-1">
              <LocationPicker
                key={`${v.warehouse_uuid}-${v.scope_type}`}
                warehouseUuid={v.warehouse_uuid}
                value={v.location_uuid}
                onChange={(uuid) => set("location_uuid", uuid)}
                onRoom={(uuid) => set("room_uuid", uuid)}
                roomOnly={v.scope_type === "room"}
                testId="count-pick"
              />
              <FieldError
                id="count-target-error"
                text={errors.room_uuid ?? errors.location_uuid}
              />
            </div>
          ) : null}

          {v.scope_type === "product" ? (
            <div className="grid gap-2 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="count-product-search">
                  {t("warehouse.generate.product_search")}
                </Label>
                <Input
                  id="count-product-search"
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  data-testid="count-product-search"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="count-product">
                  {t("warehouse.fields.product")}
                </Label>
                <select
                  id="count-product"
                  className={nativeSelectClass}
                  value={v.product_uuid}
                  data-testid="count-product"
                  onChange={(e) => set("product_uuid", e.target.value)}
                >
                  <option value="">
                    {t("warehouse.generate.pick_product")}
                  </option>
                  {(products.data?.items ?? []).map((p) => (
                    <option key={p.uuid} value={p.uuid}>
                      {p.sku} · {p.name}
                    </option>
                  ))}
                </select>
                <FieldError
                  id="count-product-error"
                  text={errors.product_uuid}
                />
              </div>
            </div>
          ) : null}

          <div className="space-y-1.5">
            <Label htmlFor="count-note">{t("warehouse.fields.note")}</Label>
            <Input
              id="count-note"
              value={v.note}
              onChange={(e) => set("note", e.target.value)}
              data-testid="count-note"
            />
            <FieldError id="count-note-error" text={errors.note} />
          </div>

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
              data-testid="count-create"
            >
              {t("warehouse.counts.create")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
