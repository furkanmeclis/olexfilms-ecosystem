"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CAMPAIGN_PAGE_SIZE,
  canCancel,
} from "@/features/campaigns/lib/campaigns";
import {
  CAMPAIGNS_PERSIST_KEY,
  campaignColumns,
} from "@/features/campaigns/components/campaigns-shared";
import {
  useCampaignMutations,
  useCampaignsList,
} from "@/features/campaigns/hooks/use-campaigns";
import { ExportMenu } from "@/features/io/components/export-menu";
import type {
  Campaign,
  CampaignListQuery,
} from "@/features/campaigns/services/campaigns.service";
import { useLocale } from "@/providers/locale-provider";

export function CampaignsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const mutations = useCampaignMutations();

  const baseColumns = useMemo(
    () =>
      campaignColumns({
        slug,
        t,
        format,
        onDuplicate: (campaign) =>
          mutations.create.mutate(
            {
              name: t("campaigns.copy_name", { name: campaign.name }),
              channels: campaign.channels,
              audience_filter: campaign.audience_filter,
            },
            {
              onSuccess: (created) => {
                toast.success(t("campaigns.toast.duplicated"));
                router.push(routes.tenant.campaigns.detail(slug, created.uuid));
              },
              onError: () => toast.error(t("campaigns.toast.failed")),
            },
          ),
        onCancel: (campaign) => {
          if (!canCancel(campaign.status)) return;
          mutations.cancel.mutate(campaign.uuid, {
            onSuccess: () => toast.success(t("campaigns.toast.cancelled")),
            onError: () => toast.error(t("campaigns.toast.failed")),
          });
        },
        onDelete: (campaign) =>
          mutations.remove.mutate(campaign.uuid, {
            onSuccess: () => toast.success(t("campaigns.toast.deleted")),
            onError: () => toast.error(t("campaigns.toast.failed")),
          }),
      }),
    [format, mutations, router, slug, t],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<Campaign>(), ...baseColumns],
    [baseColumns],
  ) as ColumnDef<Campaign>[];
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: CAMPAIGN_PAGE_SIZE,
    persistKey: CAMPAIGNS_PERSIST_KEY,
  });
  const params = listState.params as CampaignListQuery;
  const list = useCampaignsList(params);
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: params.q,
      sort: params.sort,
    }),
    [listState.filterParams, params.q, params.sort],
  );

  return (
    <EntityPage
      title={t("campaigns.title")}
      description={t("campaigns.description")}
      permission={permissions.campaigns.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("campaigns.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("campaigns.title") },
      ]}
      actions={
        <div className="flex items-center gap-2">
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.campaigns.create(slug))}
            label={t("campaigns.actions.create")}
            permission={permissions.campaigns.write}
          />
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.campaigns.approvals(slug))}
            label={t("campaigns.actions.approvals")}
            permission={permissions.campaigns.read}
          />
        </div>
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.campaigns.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("campaigns.empty_title")}
        emptyDescription={t("campaigns.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: CAMPAIGNS_PERSIST_KEY, rowSelection: true }}
        toolbarExtra={
          <>
            <ExportMenu
              exportPath="/v1/campaigns/export"
              query={exportQuery}
              jobsHref={routes.tenant.exports.root(slug)}
              formats={["xlsx", "csv", "json"]}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
