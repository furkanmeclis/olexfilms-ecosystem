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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CAMPAIGN_PAGE_SIZE,
  requiredLocales,
} from "@/features/campaigns/lib/campaigns";
import {
  CAMPAIGN_APPROVALS_PERSIST_KEY,
  CampaignBackButton,
  campaignColumns,
} from "@/features/campaigns/components/campaigns-shared";
import {
  useCampaignApprovals,
  useCampaignMutations,
  useCampaignPreview,
} from "@/features/campaigns/hooks/use-campaigns";
import type {
  Campaign,
  CampaignApprovalQuery,
} from "@/features/campaigns/services/campaigns.service";
import { useLocale } from "@/providers/locale-provider";

type Decision = "approve" | "changes" | "reject";

export function CampaignApprovalsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const [target, setTarget] = useState<Campaign | null>(null);
  const [decision, setDecision] = useState<Decision>("approve");
  const [reason, setReason] = useState("");
  const mutations = useCampaignMutations();
  const columns = useMemo(
    () => campaignColumns({ slug, t, format }),
    [format, slug, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "created_at",
    initialPageSize: CAMPAIGN_PAGE_SIZE,
    persistKey: CAMPAIGN_APPROVALS_PERSIST_KEY,
  });
  const params = listState.params as CampaignApprovalQuery;
  const approvals = useCampaignApprovals(params);
  const preview = useCampaignPreview(target?.uuid ?? "", Boolean(target));
  const reasonRequired = decision !== "approve";
  const submitDisabled = reasonRequired && !reason.trim();

  const openDecision = (next: Decision) => {
    setDecision(next);
    setReason("");
  };

  const submit = () => {
    if (!target || submitDisabled) return;
    const vars = { uuid: target.uuid, reason: reason.trim() };
    const opts = {
      onSuccess: () => {
        toast.success(t("campaigns.approvals.toast_decided"));
        setTarget(null);
        void approvals.refetch();
      },
      onError: () => toast.error(t("campaigns.toast.failed")),
    };
    if (decision === "approve") {
      mutations.approve.mutate(
        { uuid: target.uuid, reason: reason.trim() },
        opts,
      );
    } else if (decision === "changes") {
      mutations.requestChanges.mutate(vars, opts);
    } else {
      mutations.reject.mutate(vars, opts);
    }
  };

  return (
    <EntityPage
      title={t("campaigns.approvals.title")}
      description={t("campaigns.approvals.description")}
      permission={permissions.campaigns.read}
      forbiddenFallback={<ErrorState title={t("common.error_forbidden")} />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("campaigns.title"),
          href: routes.tenant.campaigns.list(slug),
        },
        { label: t("campaigns.approvals.title") },
      ]}
      actions={
        <CampaignBackButton
          href={routes.tenant.campaigns.list(slug)}
          label={t("common.back")}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={approvals.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setTarget(row)}
        isLoading={approvals.isLoading}
        isError={approvals.isError}
        onRetry={() => void approvals.refetch()}
        emptyTitle={t("campaigns.approvals.empty_title")}
        emptyDescription={t("campaigns.approvals.empty_description")}
        rowCount={approvals.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: CAMPAIGN_APPROVALS_PERSIST_KEY }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void approvals.refetch()}
            refreshDisabled={approvals.isFetching}
          />
        }
      />

      <Dialog
        open={Boolean(target)}
        onOpenChange={(open) => !open && setTarget(null)}
      >
        <DialogContent data-testid="campaign-approval-dialog">
          <DialogHeader>
            <DialogTitle>{target?.name}</DialogTitle>
            <DialogDescription>
              {t("campaigns.approvals.preview_description")}
            </DialogDescription>
          </DialogHeader>
          {target ? (
            <div className="space-y-3">
              <div className="bg-muted/40 rounded-md p-3 text-sm">
                <div>
                  {t("campaigns.preview.total", {
                    count: preview.data?.total ?? target.recipients_total,
                  })}
                </div>
                <div>
                  {t("campaigns.preview.languages")}:{" "}
                  {requiredLocales(
                    preview.data?.locales ?? [],
                    (target.contents ?? []).map((content) => content.locale),
                  ).join(", ")}
                </div>
                <div>
                  {t("campaigns.preview.excluded", {
                    count: preview.data?.excluded.total ?? 0,
                  })}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant={decision === "approve" ? "default" : "outline"}
                  onClick={() => openDecision("approve")}
                >
                  {t("campaigns.actions.approve")}
                </Button>
                <Button
                  type="button"
                  variant={decision === "changes" ? "default" : "outline"}
                  onClick={() => openDecision("changes")}
                >
                  {t("campaigns.actions.request_changes")}
                </Button>
                <Button
                  type="button"
                  variant={decision === "reject" ? "destructive" : "outline"}
                  onClick={() => openDecision("reject")}
                >
                  {t("campaigns.actions.reject")}
                </Button>
              </div>
              <div className="space-y-2">
                <Label htmlFor="campaign-decision-reason">
                  {t("campaigns.approvals.reason")}
                </Label>
                <Textarea
                  id="campaign-decision-reason"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                />
                {reasonRequired && !reason.trim() ? (
                  <p
                    className="text-destructive text-sm"
                    data-testid="campaign-reason-required"
                  >
                    {t("campaigns.validation.reason_required")}
                  </p>
                ) : null}
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              onClick={submit}
              disabled={submitDisabled}
              data-testid="campaign-decision-submit"
            >
              {t("campaigns.approvals.submit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </EntityPage>
  );
}
