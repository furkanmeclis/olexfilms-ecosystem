"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { SUPPORTED_LOCALES } from "@/config/i18n";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ReviewQuestionDialog,
  splitValues,
  type ReviewQuestionFormValues,
} from "@/features/review-questions/components/review-question-dialog";
import {
  changedLocales,
  nextSortOrder,
  questionText,
  reorderPatches,
} from "@/features/review-questions/lib/review-questions";
import {
  REVIEW_QUESTION_TARGETS,
  REVIEW_QUESTION_TYPES,
  TARGET_LABEL_KEYS,
  TYPE_LABEL_KEYS,
  reviewQuestionsService,
  type ReviewQuestion,
} from "@/features/review-questions/services/review-questions.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const REVIEW_QUESTIONS_PERSIST_KEY = "platform-review-questions-v1";

export const reviewQuestionKeys = {
  list: ["platform", "review-questions"] as const,
};

type QuestionList = { items: ReviewQuestion[] };

/**
 * Platform > Review questions (TEC-353): the admin questions under the two
 * fixed portal ratings. Client-side DataTable (the API returns the whole
 * list) with search, type / target / required / active filters, inline
 * required / active toggles and drag-to-reorder (sort_order PATCHes).
 */
export function ReviewQuestionsPage() {
  const { t, locale, format } = useLocale();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<ReviewQuestion | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const listKey = reviewQuestionKeys.list;

  const list = useQuery({
    queryKey: listKey,
    queryFn: () => reviewQuestionsService.list(),
  });
  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error)
        ? error.message
        : t("services.review_questions.toast.failed"),
    );
  const invalidate = () => queryClient.invalidateQueries({ queryKey: listKey });

  const save = useMutation({
    mutationFn: async (values: ReviewQuestionFormValues) => {
      const { input, texts } = splitValues(values);
      const saved = editing
        ? await reviewQuestionsService.update(editing.uuid, input)
        : await reviewQuestionsService.create({
            ...input,
            sort_order: nextSortOrder(items),
          });
      for (const loc of changedLocales(saved.locales, texts)) {
        await reviewQuestionsService.putLocale(
          saved.uuid,
          loc.locale,
          loc.text,
        );
      }
      return saved;
    },
    onSuccess: async () => {
      setFormOpen(false);
      appToast.success(t("services.review_questions.toast.saved"));
    },
    onError,
    // A failed locale PUT still leaves the question saved: always refetch.
    onSettled: () => invalidate(),
  });

  // Inline edit (double-click) of required / active → PATCH that flag.
  const patch = useMutation({
    mutationFn: ({
      uuid,
      input,
    }: {
      uuid: string;
      input: { is_required?: boolean; is_active?: boolean };
    }) => reviewQuestionsService.update(uuid, input),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("table.cell_saved"));
    },
    onError,
  });

  // Drag-and-drop → PATCH sort_order of every moved question (optimistic).
  const reorder = useMutation({
    mutationFn: async (rows: ReviewQuestion[]) => {
      for (const p of reorderPatches(rows)) {
        await reviewQuestionsService.update(p.uuid, {
          sort_order: p.sort_order,
        });
      }
    },
    onMutate: async (rows) => {
      await queryClient.cancelQueries({ queryKey: listKey });
      const previous = queryClient.getQueryData<QuestionList>(listKey);
      queryClient.setQueryData<QuestionList>(listKey, { items: rows });
      return { previous };
    },
    onSuccess: () => appToast.success(t("table.reorder_saved")),
    onError: (error, _rows, context) => {
      if (context?.previous) {
        queryClient.setQueryData(listKey, context.previous);
      }
      onError(error);
    },
    onSettled: () => invalidate(),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<ReviewQuestion>({
          accessorKey: "sort_order",
          labelKey: "services.review_questions.fields.sort_order",
          enableSorting: true,
          size: 90,
        }),
        createColumn<ReviewQuestion>({
          id: "text",
          accessorFn: (row) => questionText(row, locale),
          labelKey: "services.review_questions.fields.text",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {questionText(row.original, locale)}
            </span>
          ),
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "question_key",
          labelKey: "services.review_questions.fields.key",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground font-mono text-xs">
              {row.original.question_key}
            </span>
          ),
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "question_type",
          labelKey: "services.review_questions.fields.type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: REVIEW_QUESTION_TYPES.map((value) => ({
            value,
            label: t(TYPE_LABEL_KEYS[value]),
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(TYPE_LABEL_KEYS[row.original.question_type])}
            </Badge>
          ),
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "target",
          labelKey: "services.review_questions.fields.target",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: REVIEW_QUESTION_TARGETS.map((value) => ({
            value,
            label: t(TARGET_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <Badge variant="secondary">
              {t(TARGET_LABEL_KEYS[row.original.target])}
            </Badge>
          ),
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "is_required",
          labelKey: "services.review_questions.fields.required",
          enableSorting: true,
          filterVariant: "boolean",
          editVariant: "boolean",
          cell: ({ row }) =>
            row.original.is_required
              ? t("services.review_questions.required")
              : t("services.review_questions.optional"),
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "is_active",
          labelKey: "services.review_questions.fields.active",
          enableSorting: true,
          filterVariant: "boolean",
          editVariant: "boolean",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.is_active
                  ? t("services.review_questions.active")
                  : t("services.review_questions.inactive")
              }
              tone={row.original.is_active ? "success" : "default"}
            />
          ),
        }),
        createColumn<ReviewQuestion>({
          id: "translations",
          accessorFn: (row) => row.locales.length,
          labelKey: "services.review_questions.fields.translations",
          enableSorting: true,
          cell: ({ row }) =>
            `${row.original.locales.length}/${SUPPORTED_LOCALES.length}`,
        }),
        createColumn<ReviewQuestion>({
          accessorKey: "updated_at",
          labelKey: "services.review_questions.fields.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.updated_at),
        }),
        createColumn<ReviewQuestion>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("common.edit"),
                  icon: Pencil,
                  onSelect: () => {
                    setEditing(row.original);
                    setFormOpen(true);
                  },
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<ReviewQuestion, unknown>[],
    [format, locale, t],
  );

  return (
    <EntityPage
      title={t("services.review_questions.title")}
      description={t("services.review_questions.description")}
      permission={permissions.reviews.questionsManage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.review_questions.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("services.review_questions.title") },
      ]}
      actions={
        <EntityCreateButton
          label={t("services.review_questions.create")}
          onClick={() => {
            setEditing(null);
            setFormOpen(true);
          }}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("services.review_questions.empty_title")}
        emptyDescription={t("services.review_questions.empty_description")}
        initialState={{
          sorting: [{ id: "sort_order", desc: false }],
          pagination: { pageIndex: 0, pageSize: 50 },
        }}
        pageSizeOptions={[20, 50, 100]}
        features={{
          persistKey: REVIEW_QUESTIONS_PERSIST_KEY,
          rowSelection: false,
          rowReorder: true,
          inlineEdit: true,
        }}
        onRowReorder={(rows) => reorder.mutate(rows)}
        onCellEdit={({ row, columnId, value }) => {
          if (columnId !== "is_required" && columnId !== "is_active") return;
          const next = Boolean(value);
          if (next === row[columnId]) return;
          patch.mutate({ uuid: row.uuid, input: { [columnId]: next } });
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />

      <ReviewQuestionDialog
        open={formOpen}
        question={editing}
        pending={save.isPending}
        onOpenChange={setFormOpen}
        onSubmit={(values) => save.mutateAsync(values)}
      />
    </EntityPage>
  );
}
