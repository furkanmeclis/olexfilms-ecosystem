"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormSection,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  catalogKeys,
  useCatalogAccess,
} from "@/features/catalog/hooks/use-catalog-access";
import { useCategoryOptions } from "@/features/catalog/hooks/use-category-options";
import {
  productDefaults,
  productFormSchema,
  productInput,
  type ProductFormValues,
} from "@/features/catalog/lib/form";
import {
  CATALOG_UNIT_TYPES,
  catalogService,
  type CatalogProduct,
  type CatalogProductInput,
} from "@/features/catalog/services/catalog.service";
import { Markdown } from "@/features/portal/lib/markdown";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Create (uuid undefined) or edit a product; center + catalog.write only. */
export function ProductFormPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid?: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { catalog } = useCatalogAccess(slug);
  const categories = useCategoryOptions(catalog.canWrite);

  const product = useQuery({
    queryKey: catalogKeys.product(uuid ?? ""),
    queryFn: () => catalogService.getProduct(uuid ?? ""),
    enabled: Boolean(uuid) && catalog.canWrite,
  });

  const save = useMutation({
    mutationFn: (input: CatalogProductInput) =>
      uuid
        ? catalogService.updateProduct(uuid, input)
        : catalogService.createProduct(input),
    onSuccess: async (saved: CatalogProduct) => {
      await queryClient.invalidateQueries({ queryKey: catalogKeys.all });
      appToast.success(t("catalog.toast.saved"));
      router.push(routes.tenant.catalog.product(slug, saved.uuid));
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("catalog.toast.failed"),
      ),
  });

  const title = uuid
    ? t("catalog.products.edit_title")
    : t("catalog.products.create_title");
  const header = (
    <PageHeader
      title={title}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("catalog.products.title"),
          href: routes.tenant.catalog.products(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!catalog.canWrite) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.write_forbidden")}
        />
      </div>
    );
  }
  if (uuid && product.isLoading) return <Loading />;
  if (uuid && (product.isError || !product.data)) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void product.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const locked = new Set(product.data?.locked_fields ?? []);
  const unitOptions = CATALOG_UNIT_TYPES.map((u) => ({
    value: u,
    label: t(`catalog.unit_types.${u}`),
  }));
  const categoryOptions = categories.options.map(({ value, label }) => ({
    value,
    label,
  }));

  return (
    <div className="space-y-6">
      {header}
      {locked.size > 0 ? (
        <Alert>
          <AlertDescription>
            {t("catalog.products.locked_hint")}
          </AlertDescription>
        </Alert>
      ) : null}
      <AppForm<ProductFormValues>
        key={product.data?.uuid ?? "new"}
        schema={productFormSchema(t)}
        defaultValues={productDefaults(product.data)}
        onSubmit={async (values) => {
          await save.mutateAsync(productInput(values));
        }}
        className="space-y-6"
      >
        {(form) => (
          <>
            <FormSection
              id="product-general"
              title={t("catalog.products.section_general")}
              columns={2}
            >
              <AppInput
                name="sku"
                label={t("catalog.fields.sku")}
                readOnly={locked.has("sku")}
              />
              <AppInput
                name="name"
                label={t("catalog.fields.name")}
                readOnly={locked.has("name")}
              />
              <AppSelect
                name="category_uuid"
                label={t("catalog.fields.category")}
                placeholder={t("catalog.products.pick_category")}
                options={categoryOptions}
                disabled={locked.has("category_id")}
              />
              <AppSelect
                name="unit_type"
                label={t("catalog.fields.unit_type")}
                description={t("catalog.products.unit_hint")}
                options={unitOptions}
              />
              <AppInput
                name="warranty_duration_months"
                inputMode="numeric"
                label={t("catalog.fields.warranty_duration_months")}
                readOnly={locked.has("warranty_duration")}
              />
              <AppInput
                name="micron_thickness"
                inputMode="decimal"
                label={t("catalog.fields.micron_thickness")}
                readOnly={locked.has("micron_thickness")}
              />
              <AppSwitch
                name="uses_fixed_barcode"
                label={t("catalog.fields.uses_fixed_barcode")}
                description={t("catalog.products.fixed_barcode_hint")}
              />
              <AppSwitch
                name="active"
                label={t("catalog.fields.active")}
                disabled={locked.has("is_active")}
              />
            </FormSection>

            <FormSection
              id="product-images"
              title={t("catalog.fields.images")}
              description={t("catalog.products.images_hint")}
            >
              <AppTextarea
                name="images"
                label={t("catalog.fields.images")}
                rows={4}
                readOnly={locked.has("images")}
              />
            </FormSection>

            <FormSection
              id="product-description"
              title={t("catalog.fields.description_md")}
              description={t("catalog.products.markdown_hint")}
              columns={2}
            >
              <AppTextarea
                name="description_md"
                label={t("catalog.fields.description_md")}
                rows={12}
                readOnly={locked.has("description")}
              />
              <div className="space-y-2">
                <p className="text-sm font-medium">
                  {t("catalog.products.preview")}
                </p>
                <div className="bg-muted/30 min-h-40 rounded-md border p-3">
                  <Markdown source={form.watch("description_md") ?? ""} />
                </div>
              </div>
            </FormSection>

            <FormActions>
              <Button
                type="button"
                variant="outline"
                onClick={() => router.back()}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {t("common.save")}
              </Button>
            </FormActions>
          </>
        )}
      </AppForm>
    </div>
  );
}
