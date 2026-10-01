"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Trash2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityDeleteDialog } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { DistributorPricesCard } from "@/features/catalog/components/distributor-prices-card";
import { ProductPricesCard } from "@/features/catalog/components/product-prices-card";
import {
  catalogKeys,
  useCatalogAccess,
} from "@/features/catalog/hooks/use-catalog-access";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { Markdown } from "@/features/portal/lib/markdown";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * Tenant > Catalog > Product (TEC-147). Search hits of TEC-145 link here
 * (`/catalog/products/{uuid}` under the tenant shell).
 */
export function ProductDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { catalog, pricing } = useCatalogAccess(slug);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const product = useQuery({
    queryKey: catalogKeys.product(uuid),
    queryFn: () => catalogService.getProduct(uuid),
    enabled: catalog.canRead,
  });
  const remove = useMutation({
    mutationFn: () => catalogService.deleteProduct(uuid),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: catalogKeys.all });
      appToast.success(t("catalog.toast.deleted"));
      router.push(routes.tenant.catalog.products(slug));
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("catalog.toast.failed"),
      ),
  });

  const crumbs = [
    { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
    {
      label: t("catalog.products.title"),
      href: routes.tenant.catalog.products(slug),
    },
  ];

  if (!catalog.canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("catalog.forbidden")}
      />
    );
  }
  if (product.isLoading) return <Loading />;
  if (product.isError || !product.data) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("catalog.products.title")} breadcrumbs={crumbs} />
        <ErrorState
          title={t("catalog.products.not_found")}
          onRetry={() => void product.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const p = product.data;
  const images = [...p.images].sort((a, b) => a.sort - b.sort);

  return (
    <div className="space-y-6">
      <PageHeader
        title={p.name}
        description={p.sku}
        breadcrumbs={[...crumbs, { label: p.name }]}
        actions={
          catalog.canWrite ? (
            <div className="flex flex-wrap gap-2">
              <Button asChild variant="outline">
                <Link href={routes.tenant.catalog.productEdit(slug, p.uuid)}>
                  <Pencil className="size-4" />
                  {t("common.edit")}
                </Link>
              </Button>
              <Button
                type="button"
                variant="destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("common.delete")}
              </Button>
            </div>
          ) : null
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>{t("catalog.products.section_general")}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Field label={t("catalog.fields.sku")}>
              <code>{p.sku}</code>
            </Field>
            <Field label={t("catalog.fields.category")}>
              {p.category.name}
            </Field>
            <Field label={t("catalog.fields.unit_type")}>
              {t(`catalog.unit_types.${p.unit_type}`)}
            </Field>
            <Field label={t("catalog.fields.uses_fixed_barcode")}>
              {p.uses_fixed_barcode ? t("common.yes") : t("common.no")}
            </Field>
            <Field label={t("catalog.fields.warranty_duration_months")}>
              {p.warranty_duration_months === null
                ? "—"
                : format.number(p.warranty_duration_months)}
            </Field>
            <Field label={t("catalog.fields.micron_thickness")}>
              {p.micron_thickness === null
                ? "—"
                : format.number(p.micron_thickness)}
            </Field>
            <Field label={t("catalog.fields.active")}>
              <StatusChip
                label={
                  p.active
                    ? t("catalog.status.active")
                    : t("catalog.status.inactive")
                }
                tone={p.active ? "success" : "default"}
              />
            </Field>
            <Field label={t("catalog.fields.updated_at")}>
              {format.dateTime(p.updated_at)}
            </Field>
          </dl>
          {images.length ? (
            <div className="mt-4 space-y-1">
              <p className="text-muted-foreground text-xs">
                {t("catalog.fields.images")}
              </p>
              <div className="flex flex-wrap gap-1">
                {images.map((img) => (
                  <Badge key={img.key} variant="outline" className="font-mono">
                    {img.key}
                  </Badge>
                ))}
              </div>
            </div>
          ) : null}
          {p.locked_fields.length ? (
            <p className="text-muted-foreground mt-4 text-xs">
              {t("catalog.products.locked_hint")}
            </p>
          ) : null}
        </CardContent>
      </Card>

      {p.description_md.trim() ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("catalog.fields.description_md")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Markdown source={p.description_md} />
          </CardContent>
        </Card>
      ) : null}

      <ProductPricesCard productUuid={p.uuid} access={pricing} />
      <DistributorPricesCard productUuid={p.uuid} access={pricing} />

      {catalog.canWrite ? (
        <EntityDeleteDialog
          open={confirmDelete}
          entityLabel={p.name}
          softDelete={false}
          isPending={remove.isPending}
          onConfirm={() => remove.mutate()}
          onCancel={() => setConfirmDelete(false)}
        />
      ) : null}
    </div>
  );
}
