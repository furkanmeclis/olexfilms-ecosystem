"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Building2 } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityLink,
  EntityTable,
  RelatedList,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BalanceLabel, Money } from "@/features/accounting/components/shared";
import { accountingService } from "@/features/accounting/services/accounting.service";
import { ORGANIZATION_STATUS_TONE } from "@/features/organizations/constants";
import {
  organizationsService,
  type Organization,
} from "@/features/organizations/services/organizations.service";
import { ordersService } from "@/features/orders/services/orders.service";
import type { components } from "@/generated/api";
import { useCanSeePrices } from "@/hooks/use-can-see-prices";
import { isApiError, platformRequest } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type StockProduct = components["schemas"]["StockProduct"];

export const ORGANIZATION_TABS = [
  "summary",
  "children",
  "orders",
  "services",
  "warranties",
  "stock",
  "cari",
  "tasks",
] as const;
export type OrganizationTab = (typeof ORGANIZATION_TABS)[number];

/**
 * Tabs the caller may open (TEC-1217). Cari needs accounting.read and a
 * price permission: warehouse roles never see balances (TEC-1050).
 */
export function visibleOrganizationTabs(
  can: (permission: string) => boolean,
  canSeePrices: boolean,
): OrganizationTab[] {
  return ORGANIZATION_TABS.filter((tab) => {
    switch (tab) {
      case "children":
        return true;
      case "orders":
        return can(permissions.orders.read);
      case "services":
        return can(permissions.services.read);
      case "warranties":
        return can(permissions.warranties.read);
      case "stock":
        return can(permissions.stock.read);
      case "cari":
        return can(permissions.accounting.read) && canSeePrices;
      case "tasks":
        return can(permissions.tasks.read);
      default:
        return true;
    }
  });
}

const organizationKeys = {
  detail: (uuid: string) =>
    ["organizations", "tenant", "detail", uuid] as const,
  scope: ["organizations", "tenant", "directory"] as const,
};

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

/**
 * TEC-1217: organization (dealer / distributor) detail page of the tenant
 * shell — summary plus reverse-relation tabs. The organization must be in
 * the caller's organizations.read scope (GET /v1/tenant/organizations/{uuid},
 * 404 otherwise). The tab is kept in `?tab=`.
 */
export function TenantOrganizationDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canSeePrices = useCanSeePrices();
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const canRead = can(permissions.organizations.tenantRead);

  const detail = useQuery({
    queryKey: organizationKeys.detail(uuid),
    queryFn: () =>
      platformRequest<Organization>(
        "GET",
        `/v1/tenant/organizations/${encodeURIComponent(uuid)}`,
      ),
    enabled: canRead && Boolean(uuid),
  });

  const visible = visibleOrganizationTabs(can, canSeePrices);
  const requested = search.get("tab") as OrganizationTab | null;
  const tab: OrganizationTab =
    requested && visible.includes(requested) ? requested : "summary";
  const setTab = (next: string) => {
    const params = new URLSearchParams(search.toString());
    params.set("tab", next);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };

  const org = detail.data;
  const title = org?.name ?? t("organizations.tenant_detail.title");
  const header = (
    <PageHeader
      title={title}
      description={org?.type ? t(`organizations.types.${org.type}`) : undefined}
      icon={<Building2 className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("organizations.directory.title"),
          href: routes.tenant.dealers.list(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.tenant_detail.forbidden")}
        />
      </div>
    );
  }
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("organizations.tenant_detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
        />
      </div>
    );
  }
  if (!org) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("organizations.tenant_detail.loading")}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="organization-detail">
      {header}
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="flex-wrap">
          {visible.map((value) => (
            <TabsTrigger
              key={value}
              value={value}
              data-testid={`organization-tab-${value}`}
            >
              {t(`organizations.tenant_detail.tabs.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="summary" className="pt-4">
          {tab === "summary" ? <SummaryTab org={org} /> : null}
        </TabsContent>
        <TabsContent value="children" className="pt-4">
          {tab === "children" ? <ChildrenTab slug={slug} org={org} /> : null}
        </TabsContent>
        <TabsContent value="orders" className="pt-4">
          {tab === "orders" ? <OrdersTab uuid={org.uuid} /> : null}
        </TabsContent>
        <TabsContent value="services" className="pt-4">
          {tab === "services" ? (
            <RelatedList
              kind="service"
              filter={{ organization_uuid: org.uuid }}
              persistKey="organization-services-v1"
              hideColumns={["organization"]}
            />
          ) : null}
        </TabsContent>
        <TabsContent value="warranties" className="pt-4">
          {tab === "warranties" ? (
            <RelatedList
              kind="warranty"
              filter={{ organization_uuid: org.uuid }}
              persistKey="organization-warranties-v1"
              hideColumns={["organization"]}
            />
          ) : null}
        </TabsContent>
        <TabsContent value="stock" className="pt-4">
          {tab === "stock" ? <StockTab slug={slug} uuid={org.uuid} /> : null}
        </TabsContent>
        <TabsContent value="cari" className="pt-4">
          {tab === "cari" ? <CariTab slug={slug} org={org} /> : null}
        </TabsContent>
        <TabsContent value="tasks" className="pt-4">
          {tab === "tasks" ? (
            <RelatedList
              kind="task"
              filter={{ subject_organization_uuid: org.uuid }}
              persistKey="organization-tasks-v1"
              hideColumns={["subject"]}
            />
          ) : null}
        </TabsContent>
      </Tabs>
    </div>
  );
}

function SummaryTab({ org }: { org: Organization }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canDiscount = can(permissions.pricing.saleWrite);
  const location = [org.city, org.district].filter(Boolean).join(" / ");
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("organizations.tenant_detail.contact")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-3 sm:grid-cols-2">
            <Field label={t("organizations.columns.type")}>
              {org.type ? t(`organizations.types.${org.type}`) : "—"}
            </Field>
            <Field label={t("organizations.columns.status")}>
              <StatusChip
                label={t(`organizations.status.${org.status}`)}
                tone={ORGANIZATION_STATUS_TONE[org.status]}
              />
            </Field>
            <Field label={t("organizations.fields.slug")}>
              <span className="font-mono text-xs" dir="ltr">
                {org.slug}
              </span>
            </Field>
            <Field label={t("organizations.columns.parent")}>
              <EntityLink kind="organization" entity={org.parent ?? null} />
            </Field>
            <Field label={t("organizations.fields.phone")}>
              {org.phone ? (
                <a
                  href={`tel:${org.phone}`}
                  dir="ltr"
                  className="hover:underline"
                >
                  {org.phone}
                </a>
              ) : (
                "—"
              )}
            </Field>
            <Field label={t("organizations.tenant_detail.location")}>
              {location || "—"}
            </Field>
            <Field label={t("organizations.fields.address")}>
              {org.address || "—"}
            </Field>
            <Field
              label={t("organizations.tenant_detail.contract_valid_until")}
            >
              {org.contract_valid_until
                ? format.date(org.contract_valid_until)
                : "—"}
            </Field>
          </dl>
        </CardContent>
      </Card>
      {canDiscount && org.parent ? <DiscountCard uuid={org.uuid} /> : null}
    </div>
  );
}

/**
 * Discounts agreed with the organization (TEC-1064): only for a caller with
 * pricing.sale.write directly above it; anything else (403/404) hides it.
 */
function DiscountCard({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const discount = useQuery({
    queryKey: ["organizations", "tenant", "discount", uuid],
    queryFn: () => ordersService.getDiscount(uuid),
    retry: false,
  });
  if (!discount.data) return null;
  const pct = (v: string | null) => (v === null ? "—" : `%${v}`);
  return (
    <Card data-testid="organization-discount">
      <CardHeader>
        <CardTitle className="text-base">
          {t("organizations.tenant_detail.discount")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid gap-3 sm:grid-cols-2">
          <Field label={t("organizations.tenant_detail.standard_discount")}>
            {pct(discount.data.standard_discount_pct)}
          </Field>
          <Field label={t("organizations.tenant_detail.first_order_discount")}>
            {pct(discount.data.first_order_discount_pct)}
          </Field>
        </dl>
      </CardContent>
    </Card>
  );
}

/**
 * Organizations directly under this one, from the caller's scope list (one
 * page of at most 200, so the table works in the browser).
 */
function ChildrenTab({ slug, org }: { slug: string; org: Organization }) {
  const { t } = useLocale();
  const router = useRouter();
  const list = useQuery({
    queryKey: organizationKeys.scope,
    queryFn: () => organizationsService.listInScope(),
    staleTime: 5 * 60_000,
  });
  const rows = useMemo(
    () => (list.data ?? []).filter((o) => o.parent?.uuid === org.uuid),
    [list.data, org.uuid],
  );
  const columns = useMemo(
    () =>
      [
        createColumn<Organization>({
          accessorKey: "name",
          labelKey: "organizations.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <EntityLink
              kind="organization"
              entity={row.original}
              className="font-medium"
            />
          ),
        }),
        createColumn<Organization>({
          accessorKey: "type",
          labelKey: "organizations.columns.type",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.type
              ? t(`organizations.types.${row.original.type}`)
              : "—",
        }),
        createColumn<Organization>({
          id: "location",
          accessorFn: (o) => [o.city, o.district].filter(Boolean).join(" / "),
          labelKey: "organizations.tenant_detail.location",
          enableSorting: true,
          cell: ({ getValue }) => String(getValue() || "—"),
        }),
        createColumn<Organization>({
          accessorKey: "status",
          labelKey: "organizations.columns.status",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`organizations.status.${row.original.status}`)}
              tone={ORGANIZATION_STATUS_TONE[row.original.status]}
            />
          ),
        }),
      ] as ColumnDef<Organization, unknown>[],
    [t],
  );
  return (
    <EntityTable
      columns={columns}
      data={rows}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      onRowClick={(row) =>
        router.push(routes.tenant.organizations.detail(slug, row.uuid))
      }
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("organizations.tenant_detail.children_empty")}
      features={{ persistKey: "organization-children-v1", rowSelection: false }}
    />
  );
}

function OrdersTab({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const [side, setSide] = useState<"buyer" | "seller">("buyer");
  return (
    <div className="space-y-3">
      <div className="flex gap-2" role="group">
        {(["buyer", "seller"] as const).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={side === value ? "default" : "outline"}
            onClick={() => setSide(value)}
            data-testid={`organization-orders-${value}`}
          >
            {t(`organizations.tenant_detail.orders_as_${value}`)}
          </Button>
        ))}
      </div>
      {side === "buyer" ? (
        <RelatedList
          key="buyer"
          kind="order"
          filter={{ buyer_org_uuid: uuid }}
          persistKey="organization-orders-buyer-v1"
          hideColumns={["buyer"]}
        />
      ) : (
        <RelatedList
          key="seller"
          kind="order"
          filter={{ seller_org_uuid: uuid }}
          persistKey="organization-orders-seller-v1"
          hideColumns={["seller"]}
        />
      )}
    </div>
  );
}

const STOCK_PERSIST_KEY = "organization-stock-v1";

/** Product stock on hand of the organization (no prices). */
function StockTab({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<StockProduct>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "organizations.tenant_detail.stock_product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span>
              <EntityLink kind="product" entity={row.original.product} />{" "}
              <span
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </span>
            </span>
          ),
        }),
        createColumn<StockProduct>({
          accessorKey: "quantity",
          labelKey: "organizations.tenant_detail.stock_quantity",
          enableSorting: true,
          cell: ({ row }) => format.number(row.original.quantity),
        }),
        createColumn<StockProduct>({
          accessorKey: "meters",
          labelKey: "organizations.tenant_detail.stock_meters",
          enableSorting: true,
          cell: ({ row }) => format.number(Number(row.original.meters)),
        }),
        createColumn<StockProduct>({
          accessorKey: "updated_at",
          labelKey: "organizations.tenant_detail.stock_updated_at",
          enableSorting: true,
          cell: ({ row }) => format.date(row.original.updated_at),
        }),
      ] as ColumnDef<StockProduct, unknown>[],
    [format],
  );
  const list = useServerListState({
    columns,
    initialSort: "product",
    initialPageSize: 10,
    persistKey: STOCK_PERSIST_KEY,
  });
  const result = useQuery({
    queryKey: ["organizations", "tenant", "stock", uuid, list.params],
    queryFn: () =>
      platformRequest<{ items: StockProduct[]; total: number }>(
        "GET",
        `/v1/stock/organizations/${encodeURIComponent(uuid)}/products`,
        { query: list.params },
      ),
  });
  return (
    <div className="space-y-3">
      <EntityTable
        columns={columns}
        data={result.data?.items ?? []}
        getRowId={(row) => row.product.uuid}
        isLoading={result.isLoading}
        isError={result.isError}
        onRetry={() => void result.refetch()}
        emptyTitle={t("organizations.tenant_detail.stock_empty")}
        rowCount={result.data?.total ?? 0}
        state={list.tableState}
        features={{ persistKey: STOCK_PERSIST_KEY }}
      />
      <Button asChild variant="outline" size="sm">
        <Link
          href={`${routes.tenant.stock.root(slug)}?organization=${encodeURIComponent(uuid)}`}
        >
          {t("organizations.tenant_detail.stock_open")}
        </Link>
      </Button>
    </div>
  );
}

/**
 * The organization's cari in the active book: looked up by name, matched
 * on the counterparty uuid.
 */
function CariTab({ slug, org }: { slug: string; org: Organization }) {
  const { t } = useLocale();
  const cari = useQuery({
    queryKey: ["organizations", "tenant", "cari", org.uuid],
    queryFn: async () => {
      const page = await accountingService.listCari({ q: org.name, limit: 50 });
      return (
        (page.items ?? []).find((c) => c.counterparty.uuid === org.uuid) ?? null
      );
    },
  });
  if (cari.isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("organizations.tenant_detail.loading")}
      </p>
    );
  }
  if (cari.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void cari.refetch()}
      />
    );
  }
  const c = cari.data;
  if (!c) {
    return (
      <p
        className="text-muted-foreground text-sm"
        data-testid="organization-cari-none"
      >
        {t("organizations.tenant_detail.cari_none")}
      </p>
    );
  }
  return (
    <Card data-testid="organization-cari">
      <CardHeader>
        <CardTitle className="text-base">
          {t("organizations.tenant_detail.cari_balance")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-3 text-lg font-semibold">
          <Money amount={c.balance} currency={c.currency} />
          <BalanceLabel balance={c.balance} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button asChild size="sm">
            <Link href={routes.tenant.accounting.cariDetail(slug, c.uuid)}>
              {t("organizations.tenant_detail.cari_open")}
            </Link>
          </Button>
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.accounting.statement(slug, c.uuid)}>
              {t("accounting.cari.statement")}
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
