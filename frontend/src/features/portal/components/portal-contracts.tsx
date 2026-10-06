"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, FileSignature, Loader2 } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { PortalPage } from "@/features/portal/components/portal-page";
import {
  portalApi,
  portalContractPdfUrl,
  type PortalContract,
} from "@/features/portal/lib/portal-client";
import { PORTAL_CONTRACT_PAGE_SIZE } from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

export const PORTAL_CONTRACTS_PERSIST_KEY = "portal-contracts-v1";

/**
 * Portal > My contracts (TEC-245, TEC-292): the executed vehicle intake
 * contracts of the user (GET /v1/portal/contracts) with their PDF
 * (GET /v1/portal/contracts/{uuid}/pdf). The endpoint pages only (newest
 * execution first, no sort / search / filters), so the table is server
 * paged with layout features. Customer and fleet sessions see the same
 * read-only list: the page has no write.
 */
export function PortalContracts() {
  const { t, format } = useLocale();

  const columns = useMemo<ColumnDef<PortalContract, unknown>[]>(
    () => [
      createColumn<PortalContract>({
        accessorKey: "contract_no",
        labelKey: "portal.contracts.col.contract_no",
        enableSorting: false,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className="font-mono font-medium"
            dir="ltr"
            data-testid="portal-contract-no"
          >
            #{row.original.contract_no}
          </span>
        ),
      }),
      createColumn<PortalContract>({
        id: "vehicle",
        labelKey: "portal.contracts.col.vehicle",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => <ContractVehicle contract={row.original} />,
      }),
      createColumn<PortalContract>({
        id: "dealer",
        accessorFn: (c) => c.organization.name,
        labelKey: "portal.contracts.col.dealer",
        enableSorting: false,
      }),
      createColumn<PortalContract>({
        id: "signed_at",
        accessorFn: (c) => c.executed_at ?? c.created_at,
        labelKey: "portal.contracts.col.signed_at",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.date(row.original.executed_at ?? row.original.created_at)}
          </span>
        ),
      }),
      createColumn<PortalContract>({
        id: "service",
        accessorFn: (c) => c.service.service_no,
        labelKey: "portal.contracts.col.service",
        enableSorting: false,
        cell: ({ row }) => (
          <Link
            href={routes.portal.service(row.original.service.uuid)}
            className="font-mono text-sm underline-offset-4 hover:underline"
            dir="ltr"
            data-testid="portal-contract-service"
          >
            {row.original.service.service_no}
          </Link>
        ),
      }),
      createColumn<PortalContract>({
        id: "pdf",
        labelKey: "portal.contracts.col.pdf",
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => <ContractPdf contract={row.original} />,
      }),
    ],
    [format],
  );

  const listState = useServerListState({
    columns,
    initialSort: null,
    initialPageSize: PORTAL_CONTRACT_PAGE_SIZE,
    persistKey: PORTAL_CONTRACTS_PERSIST_KEY,
  });
  const { limit, offset } = listState.params;

  const list = useQuery({
    queryKey: ["portal", "contracts", limit, offset],
    queryFn: () => portalApi.listContracts(limit, offset),
    placeholderData: keepPreviousData,
  });
  const total = list.data?.total ?? 0;
  const empty = list.isSuccess && total === 0;

  return (
    <PortalPage
      title={t("portal.contracts.title")}
      icon={<FileSignature className="size-6" />}
      testId="portal-contracts-page"
    >
      {empty ? (
        <Card>
          <CardContent
            className="flex flex-col items-center gap-2 py-10 text-center"
            data-testid="portal-contracts-empty"
          >
            <FileSignature className="text-muted-foreground size-10" />
            <p className="font-medium">{t("portal.contracts.empty_title")}</p>
            <p className="text-muted-foreground max-w-sm text-sm">
              {t("portal.contracts.empty_hint")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div data-testid="portal-contracts">
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(c) => c.contract_uuid}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("portal.contracts.empty_title")}
            emptyDescription={t("portal.contracts.empty_hint")}
            rowCount={total}
            state={listState.tableState}
            features={{
              persistKey: PORTAL_CONTRACTS_PERSIST_KEY,
              sorting: false,
              globalFilter: false,
              columnFilters: false,
              facetedFilters: false,
              rowSelection: false,
            }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void list.refetch()}
                refreshDisabled={list.isFetching}
              />
            }
          />
        </div>
      )}
    </PortalPage>
  );
}

function ContractVehicle({ contract }: { contract: PortalContract }) {
  const name = [contract.car_brand_name, contract.car_model_name]
    .filter(Boolean)
    .join(" ");
  return (
    <span className="flex flex-wrap items-baseline gap-x-2">
      <span>
        {name}
        {contract.model_year ? ` (${contract.model_year})` : ""}
      </span>
      {contract.plate ? (
        <span
          className="text-muted-foreground font-mono text-sm tracking-wider"
          dir="ltr"
        >
          {contract.plate}
        </span>
      ) : null}
    </span>
  );
}

/** "Download PDF" link, or a disabled "preparing" button until it renders. */
function ContractPdf({ contract }: { contract: PortalContract }) {
  const { t } = useLocale();
  if (!contract.pdf_ready) {
    return (
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled
        title={t("portal.contracts.pdf_preparing_hint")}
        data-testid="portal-contract-pdf-pending"
      >
        <Loader2 className="size-4 animate-spin" aria-hidden />
        {t("portal.contracts.pdf_preparing")}
      </Button>
    );
  }
  return (
    <Button asChild variant="outline" size="sm">
      <a
        href={portalContractPdfUrl(contract.contract_uuid)}
        download={`contract-${contract.contract_no}.pdf`}
        data-testid="portal-contract-pdf"
      >
        <Download className="size-4" aria-hidden />
        {t("portal.contracts.download_pdf")}
      </a>
    </Button>
  );
}
