"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Eye, X } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ShowcasePreview } from "@/features/showcase/components/showcase-preview";
import {
  showcaseKeys,
  showcaseService,
  type Showcase,
  type ShowcaseReviewItem,
  type ShowcaseReviewQuery,
  type ShowcaseStatus,
} from "@/features/showcase/services/showcase.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const PLATFORM_SHOWCASES_PERSIST_KEY = "platform-showcases-v1";

const STATUSES: ShowcaseStatus[] = [
  "draft",
  "pending_review",
  "published",
  "rejected",
];

function tone(status: ShowcaseStatus) {
  if (status === "published") return "success";
  if (status === "pending_review") return "warning";
  if (status === "rejected") return "danger";
  return "default";
}

export function PlatformShowcasesPage() {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [selectedOrg, setSelectedOrg] = useState<string | null>(null);
  const [rejectNote, setRejectNote] = useState("");
  const [noteError, setNoteError] = useState(false);

  const columns = useMemo<ColumnDef<ShowcaseReviewItem, unknown>[]>(
    () => [
      createColumn<ShowcaseReviewItem>({
        id: "name",
        accessorFn: (row) => row.organization.name,
        labelKey: "showcase.queue.columns.organization",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div>
            <p className="font-medium">{row.original.organization.name}</p>
            <p className="text-muted-foreground text-xs">
              {row.original.organization.city}
            </p>
          </div>
        ),
      }),
      createColumn<ShowcaseReviewItem>({
        id: "city",
        accessorFn: (row) => row.organization.city,
        labelKey: "showcase.queue.columns.city",
        enableSorting: false,
        filterVariant: "faceted",
        param: "q",
        cell: ({ row }) => row.original.organization.city,
      }),
      createColumn<ShowcaseReviewItem>({
        accessorKey: "status",
        labelKey: "showcase.queue.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        filterOptions: STATUSES.map((value) => ({
          value,
          label: value,
          labelKey: `showcase.status.${value}`,
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(`showcase.status.${row.original.status}`)}
            tone={tone(row.original.status)}
          />
        ),
      }),
      createColumn<ShowcaseReviewItem>({
        accessorKey: "updated_at",
        labelKey: "showcase.queue.columns.updated_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "updated",
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }),
      createColumn<ShowcaseReviewItem>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => {
          const actions: EntityRowAction[] = [
            {
              id: "inspect",
              label: t("showcase.queue.inspect"),
              icon: Eye,
              onSelect: () => setSelectedOrg(row.original.organization.uuid),
            },
          ];
          return <EntityRowActions actions={actions} />;
        },
      }),
    ],
    [format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-updated_at",
    initialPageSize: 20,
    persistKey: PLATFORM_SHOWCASES_PERSIST_KEY,
  });
  const params = listState.params as ShowcaseReviewQuery;
  const list = useQuery({
    queryKey: showcaseKeys.review(params),
    queryFn: () => showcaseService.reviewList(params),
  });
  const preview = useQuery({
    queryKey: selectedOrg
      ? showcaseKeys.platform(selectedOrg)
      : ["showcase", "platform", "none"],
    queryFn: () => showcaseService.platformGet(selectedOrg ?? ""),
    enabled: Boolean(selectedOrg),
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: showcaseKeys.all });
  };
  const onError = (err: unknown) =>
    appToast.error(isApiError(err) ? err.message : t("common.error_generic"));
  const review = useMutation({
    mutationFn: ({
      showcase,
      decision,
      note,
    }: {
      showcase: Showcase;
      decision: "approve" | "reject";
      note?: string;
    }) =>
      showcaseService.review(showcase.organization.uuid, {
        decision,
        note,
      }),
    onSuccess: () => {
      appToast.success(t("showcase.queue.reviewed"));
      setSelectedOrg(null);
      setRejectNote("");
      setNoteError(false);
      refresh();
    },
    onError,
  });

  return (
    <EntityPage
      title={t("showcase.queue.title")}
      description={t("showcase.queue.description")}
      permission={permissions.showcase.review}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("showcase.queue.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("showcase.queue.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setSelectedOrg(row.organization.uuid)}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("showcase.queue.empty_title")}
        emptyDescription={t("showcase.queue.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: PLATFORM_SHOWCASES_PERSIST_KEY,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />

      <Dialog
        open={Boolean(selectedOrg)}
        onOpenChange={(open) => !open && setSelectedOrg(null)}
      >
        <DialogContent className="max-w-5xl">
          <DialogHeader>
            <DialogTitle>{t("showcase.queue.preview")}</DialogTitle>
          </DialogHeader>
          {preview.data ? (
            <ShowcasePreview showcase={preview.data} draft />
          ) : null}
          <div className="space-y-2">
            <Textarea
              value={rejectNote}
              onChange={(e) => {
                setRejectNote(e.target.value);
                setNoteError(false);
              }}
              placeholder={t("showcase.queue.reject_note")}
              aria-invalid={noteError}
            />
            {noteError ? (
              <p className="text-destructive text-sm" role="alert">
                {t("showcase.queue.reject_note_required")}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                if (!preview.data) return;
                if (!rejectNote.trim()) {
                  setNoteError(true);
                  return;
                }
                review.mutate({
                  showcase: preview.data,
                  decision: "reject",
                  note: rejectNote.trim(),
                });
              }}
              disabled={
                review.isPending || preview.data?.status !== "pending_review"
              }
            >
              <X className="size-4" />
              {t("showcase.queue.reject")}
            </Button>
            <Button
              type="button"
              onClick={() =>
                preview.data &&
                review.mutate({ showcase: preview.data, decision: "approve" })
              }
              disabled={
                review.isPending || preview.data?.status !== "pending_review"
              }
            >
              <Check className="size-4" />
              {t("showcase.queue.approve")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </EntityPage>
  );
}
