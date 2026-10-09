"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DateTimePicker } from "@/components/ui/date-time-picker";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CAMPAIGN_PAGE_SIZE,
  campaignStatusTone,
  toZonedInput,
} from "@/features/campaigns/lib/campaigns";
import {
  CAMPAIGN_RECIPIENTS_PERSIST_KEY,
  CampaignBackButton,
  ChannelBadges,
  StatCards,
  recipientColumns,
} from "@/features/campaigns/components/campaigns-shared";
import {
  campaignRecipientsExportPath,
  type CampaignRecipientQuery,
} from "@/features/campaigns/services/campaigns.service";
import {
  useCampaign,
  useCampaignMutations,
  useCampaignRecipients,
} from "@/features/campaigns/hooks/use-campaigns";
import { ExportMenu } from "@/features/io/components/export-menu";
import { StatusChip } from "@/components/common/status-chip";
import { useLocale } from "@/providers/locale-provider";

export function CampaignDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const detail = useCampaign(uuid);
  const mutations = useCampaignMutations();
  const [scheduledAt, setScheduledAt] = useState("");
  const columns = useMemo(() => recipientColumns({ t, format }), [format, t]);
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: CAMPAIGN_PAGE_SIZE,
    persistKey: CAMPAIGN_RECIPIENTS_PERSIST_KEY,
  });
  const params = listState.params as CampaignRecipientQuery;
  const recipients = useCampaignRecipients(uuid, params);
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      sort: params.sort,
    }),
    [listState.filterParams, params.sort],
  );

  const campaign = detail.data;
  const stats = campaign ? StatCards({ campaign }) : null;

  return (
    <EntityPage
      title={campaign?.name ?? t("campaigns.detail.title")}
      description={campaign ? campaign.organization_name : undefined}
      permission={permissions.campaigns.read}
      forbiddenFallback={<ErrorState title={t("common.error_forbidden")} />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("campaigns.title"),
          href: routes.tenant.campaigns.list(slug),
        },
        { label: campaign?.name ?? uuid },
      ]}
      actions={
        <CampaignBackButton
          href={routes.tenant.campaigns.list(slug)}
          label={t("common.back")}
        />
      }
    >
      {detail.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          retryLabel={t("common.retry")}
          onRetry={() => void detail.refetch()}
        />
      ) : !campaign ? (
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      ) : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip
              label={t(`campaigns.status.${campaign.status}`)}
              tone={campaignStatusTone(campaign.status)}
            />
            <ChannelBadges channels={campaign.channels} t={t} />
          </div>
          {stats ? (
            <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
              {stats.items.map(([key, value]) => (
                <Card key={key}>
                  <CardContent className="p-4">
                    <div className="text-muted-foreground text-xs">
                      {t(key)}
                    </div>
                    <div className="text-xl font-semibold">
                      {value.toLocaleString()}
                    </div>
                  </CardContent>
                </Card>
              ))}
              <Card>
                <CardContent className="p-4">
                  <div className="text-muted-foreground text-xs">
                    {t("campaigns.stats.success_rate")}
                  </div>
                  <div className="text-xl font-semibold">
                    {stats.successRate === null ? "—" : `${stats.successRate}%`}
                  </div>
                </CardContent>
              </Card>
            </div>
          ) : null}

          <Card>
            <CardHeader>
              <CardTitle>{t("campaigns.schedule.title")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap items-end gap-3">
              <div className="space-y-2">
                <Label htmlFor="campaign-detail-schedule">
                  {t("campaigns.fields.scheduled_at")}
                </Label>
                <DateTimePicker
                  id="campaign-detail-schedule"
                  className="w-auto min-w-56"
                  value={
                    scheduledAt ||
                    (campaign.scheduled_at
                      ? toZonedInput(campaign.scheduled_at, campaign.timezone)
                      : "")
                  }
                  onChange={setScheduledAt}
                />
              </div>
              <Button
                type="button"
                disabled={!scheduledAt || mutations.schedule.isPending}
                onClick={() =>
                  mutations.schedule.mutate(
                    { uuid, scheduledAt },
                    {
                      onSuccess: () => {
                        toast.success(t("campaigns.toast.scheduled"));
                        void detail.refetch();
                      },
                      onError: () => toast.error(t("campaigns.toast.failed")),
                    },
                  )
                }
              >
                {t("campaigns.actions.schedule")}
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("campaigns.timeline.title")}</CardTitle>
            </CardHeader>
            <CardContent>
              {(campaign.events ?? []).length ? (
                <ol className="space-y-3">
                  {(campaign.events ?? []).map((event) => (
                    <li key={event.uuid} className="border-s ps-3">
                      <div className="text-sm font-medium">
                        {t(`campaigns.event.${event.event_type}`)}
                      </div>
                      <div className="text-muted-foreground text-xs">
                        {format.dateTime(event.created_at)}
                      </div>
                      {event.reason ? (
                        <p className="mt-1 text-sm">{event.reason}</p>
                      ) : null}
                    </li>
                  ))}
                </ol>
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("campaigns.timeline.empty")}
                </p>
              )}
            </CardContent>
          </Card>

          <EntityTable
            columns={columns}
            data={recipients.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={recipients.isLoading}
            isError={recipients.isError}
            onRetry={() => void recipients.refetch()}
            emptyTitle={t("campaigns.recipients.empty_title")}
            emptyDescription={t("campaigns.recipients.empty_description")}
            rowCount={recipients.data?.total ?? 0}
            state={listState.tableState}
            features={{ persistKey: CAMPAIGN_RECIPIENTS_PERSIST_KEY }}
            toolbarExtra={
              <>
                <ExportMenu
                  exportPath={campaignRecipientsExportPath(uuid)}
                  query={exportQuery}
                  jobsHref={routes.tenant.exports.root(slug)}
                  formats={["xlsx", "csv", "json"]}
                />
                <EntityToolbar
                  onRefresh={() => {
                    void recipients.refetch();
                    void detail.refetch();
                  }}
                  refreshDisabled={recipients.isFetching || detail.isFetching}
                />
              </>
            }
          />
        </div>
      )}
    </EntityPage>
  );
}
