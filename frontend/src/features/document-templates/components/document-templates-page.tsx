"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useDocumentTemplates } from "@/features/document-templates/hooks/use-document-templates";
import {
  DOCUMENT_KINDS,
  DOCUMENT_LANGUAGES,
  documentLanguage,
  type DocumentKind,
  type DocumentTemplate,
  type ListDocumentTemplatesParams,
} from "@/features/document-templates/services/document-templates.service";
import { useLocale } from "@/providers/locale-provider";

export const DOCUMENT_TEMPLATES_PERSIST_KEY = "platform-document-templates-v1";

const DOCUMENT_STATUSES = ["draft", "active", "superseded"] as const;

/**
 * Status filter → `status` CSV. The list shows current versions only
 * (`current` defaults to true), so asking for superseded versions also
 * sends `current=false`.
 */
export function documentStatusParams(value: unknown) {
  const list = (Array.isArray(value) ? value : [])
    .map((item) => String(item))
    .filter(Boolean);
  return {
    status: list.length ? list.join(",") : undefined,
    current: list.includes("superseded") ? "false" : undefined,
  };
}

export function statusVariant(status: DocumentTemplate["status"]) {
  if (status === "active") return "success" as const;
  if (status === "draft") return "warning" as const;
  return "outline" as const;
}

export function DocumentTemplatesPage() {
  const { t, format } = useLocale();
  const router = useRouter();
  const [kind, setKind] = useState<DocumentKind>("service");
  const [language, setLanguage] = useState<string>("de");

  const columns = useMemo<ColumnDef<DocumentTemplate, unknown>[]>(
    () => [
      createColumn<DocumentTemplate>({
        accessorKey: "kind",
        labelKey: "documents.fields.kind",
        enableSorting: true,
        filterVariant: "faceted",
        param: "kind",
        filterOptions: DOCUMENT_KINDS.map((value) => ({
          value,
          labelKey: `documents.kinds.${value}`,
          label: value,
        })),
        cell: ({ row }) => t(`documents.kinds.${row.original.kind}`),
      }),
      createColumn<DocumentTemplate>({
        accessorKey: "language",
        labelKey: "documents.fields.language",
        enableSorting: true,
        filterVariant: "faceted",
        param: "language",
        filterOptions: DOCUMENT_LANGUAGES.map((value) => ({
          value,
          labelKey: `documents.languages.${value}`,
          label: value,
        })),
        cell: ({ row }) =>
          t(`documents.languages.${documentLanguage(row.original.language)}`),
      }),
      createColumn<DocumentTemplate>({
        id: "brand",
        accessorFn: (row) => row.brand_slug ?? "",
        labelKey: "documents.fields.brand",
        enableSorting: false,
        filterVariant: "text",
        param: "brand",
        cell: ({ row }) =>
          row.original.brand_slug ?? (
            <span className="text-muted-foreground">
              {t("documents.brand_default")}
            </span>
          ),
      }),
      createColumn<DocumentTemplate>({
        accessorKey: "name",
        labelKey: "documents.fields.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<DocumentTemplate>({
        accessorKey: "version",
        labelKey: "documents.fields.version",
        enableSorting: true,
        cell: ({ row }) => `v${row.original.version}`,
      }),
      createColumn<DocumentTemplate>({
        accessorKey: "status",
        labelKey: "documents.fields.status",
        enableSorting: false,
        filterVariant: "faceted",
        param: "status",
        paramFormat: documentStatusParams,
        gridSecondary: true,
        filterOptions: DOCUMENT_STATUSES.map((value) => ({
          value,
          labelKey: `documents.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>
            {t(`documents.status.${row.original.status}`)}
          </Badge>
        ),
      }),
      createColumn<DocumentTemplate>({
        accessorKey: "updated_at",
        labelKey: "documents.fields.updated_at",
        enableSorting: true,
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }),
      createColumn<DocumentTemplate>({
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
                label: t("documents.actions.edit"),
                icon: Pencil,
                onSelect: () =>
                  router.push(
                    routes.platform.documentTemplates.edit(row.original.uuid),
                  ),
              },
            ]}
          />
        ),
      }),
    ],
    [format, router, t],
  );

  // Backend default sort (kind, platform default first, language, newest
  // version) until the user picks a column.
  const listState = useServerListState({
    columns,
    initialSort: null,
    initialPageSize: 50,
    persistKey: DOCUMENT_TEMPLATES_PERSIST_KEY,
  });
  const listParams = listState.params as ListDocumentTemplatesParams;
  const query = useDocumentTemplates(listParams);

  return (
    <EntityPage
      title={t("documents.title")}
      description={t("documents.description")}
      permission={permissions.documentTemplates.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("documents.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("documents.title") },
      ]}
    >
      <PermissionGuard permission={permissions.documentTemplates.write}>
        <div className="mb-6 flex flex-wrap items-end gap-3 rounded-lg border p-4">
          <div className="grid gap-1">
            <span className="text-muted-foreground text-xs">
              {t("documents.fields.kind")}
            </span>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as DocumentKind)}
            >
              <SelectTrigger className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DOCUMENT_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`documents.kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1">
            <span className="text-muted-foreground text-xs">
              {t("documents.fields.language")}
            </span>
            <Select value={language} onValueChange={setLanguage}>
              <SelectTrigger className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DOCUMENT_LANGUAGES.map((l) => (
                  <SelectItem key={l} value={l}>
                    {t(`documents.languages.${l}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button
            onClick={() =>
              router.push(
                `${routes.platform.documentTemplates.root}/new?kind=${kind}&language=${language}`,
              )
            }
          >
            {t("documents.actions.new_language")}
          </Button>
        </div>
      </PermissionGuard>

      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.platform.documentTemplates.edit(row.uuid))
        }
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("documents.load_failed")}
        onRetry={() => void query.refetch()}
        rowCount={query.data?.total ?? 0}
        state={listState.tableState}
        pageSizeOptions={[20, 50, 100]}
        features={{
          persistKey: DOCUMENT_TEMPLATES_PERSIST_KEY,
          // No bulk endpoint for templates.
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
