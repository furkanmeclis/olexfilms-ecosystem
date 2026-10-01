"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityPage } from "@/components/entity";
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
  type DocumentKind,
  type DocumentTemplate,
} from "@/features/document-templates/services/document-templates.service";
import { useLocale } from "@/providers/locale-provider";

export function statusVariant(status: DocumentTemplate["status"]) {
  if (status === "active") return "success" as const;
  if (status === "draft") return "warning" as const;
  return "outline" as const;
}

export function DocumentTemplatesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const query = useDocumentTemplates({ current: true, limit: 100 });
  const [kind, setKind] = useState<DocumentKind>("service");
  const [language, setLanguage] = useState<string>("de");

  const byKind = useMemo(() => {
    const map = new Map<string, DocumentTemplate[]>();
    for (const row of query.data?.items ?? []) {
      const list = map.get(row.kind) ?? [];
      list.push(row);
      map.set(row.kind, list);
    }
    return map;
  }, [query.data]);

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

      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("documents.load_failed")}
        />
      ) : null}

      <div className="grid gap-6">
        {DOCUMENT_KINDS.map((k) => (
          <section key={k} className="rounded-lg border">
            <h2 className="border-b px-4 py-3 text-sm font-semibold">
              {t(`documents.kinds.${k}`)}
            </h2>
            <table className="w-full text-sm">
              <thead className="text-muted-foreground text-start text-xs">
                <tr>
                  <th className="px-4 py-2 text-start font-medium">
                    {t("documents.fields.language")}
                  </th>
                  <th className="px-4 py-2 text-start font-medium">
                    {t("documents.fields.brand")}
                  </th>
                  <th className="px-4 py-2 text-start font-medium">
                    {t("documents.fields.name")}
                  </th>
                  <th className="px-4 py-2 text-start font-medium">
                    {t("documents.fields.version")}
                  </th>
                  <th className="px-4 py-2 text-start font-medium">
                    {t("documents.fields.status")}
                  </th>
                  <th className="px-4 py-2" />
                </tr>
              </thead>
              <tbody>
                {(byKind.get(k) ?? []).map((row) => (
                  <tr key={row.uuid} className="border-t">
                    <td className="px-4 py-2">
                      {t(`documents.languages.${row.language}`)}
                    </td>
                    <td className="px-4 py-2">
                      {row.brand_slug ?? t("documents.brand_default")}
                    </td>
                    <td className="px-4 py-2">{row.name}</td>
                    <td className="px-4 py-2">v{row.version}</td>
                    <td className="px-4 py-2">
                      <Badge variant={statusVariant(row.status)}>
                        {t(`documents.status.${row.status}`)}
                      </Badge>
                    </td>
                    <td className="px-4 py-2 text-end">
                      <Button asChild size="sm" variant="outline">
                        <Link
                          href={routes.platform.documentTemplates.edit(
                            row.uuid,
                          )}
                        >
                          {t("documents.actions.edit")}
                        </Link>
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        ))}
      </div>
    </EntityPage>
  );
}
