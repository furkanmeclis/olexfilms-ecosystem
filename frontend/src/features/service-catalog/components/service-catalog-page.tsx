"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { modulesService } from "@/features/modules/services/modules.service";
import { moduleLevelLabel, moduleName } from "@/features/modules/lib/labels";
import {
  SERVICE_CATALOG_CATEGORIES,
  SERVICE_CATALOG_RECURRENCES,
  serviceCatalogKeys,
  serviceCatalogService,
  type ContractTemplate,
  type DistributorOption,
  type PlatformModule,
  type ServiceCatalogCategory,
  type ServiceCatalogItem,
  type ServiceCatalogItemInput,
  type ServiceCatalogOverride,
  type ServiceCatalogRecurrence,
} from "@/features/service-catalog/services/service-catalog.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Mode = "platform" | "tenant";

type FormState = {
  uuid?: string;
  name: string;
  description: string;
  category: ServiceCatalogCategory;
  default_price: string;
  currency: string;
  recurrence: ServiceCatalogRecurrence;
  cancellation_fee: string;
  contract_template_id: string;
  is_active: boolean;
  modules: string[];
};

const emptyForm: FormState = {
  name: "",
  description: "",
  category: "training",
  default_price: "",
  currency: "EUR",
  recurrence: "one_time",
  cancellation_fee: "0",
  contract_template_id: "",
  is_active: true,
  modules: [],
};

function moneyValue(value?: string | null) {
  if (!value) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function formFromItem(item: ServiceCatalogItem): FormState {
  return {
    uuid: item.uuid,
    name: item.name,
    description: item.description,
    category: item.category,
    default_price: item.default_price ?? "",
    currency: item.currency,
    recurrence: item.recurrence,
    cancellation_fee: item.cancellation_fee ?? "0",
    contract_template_id: item.contract_template_id
      ? String(item.contract_template_id)
      : "",
    is_active: item.is_active,
    modules: item.modules ?? [],
  };
}

function inputFromForm(form: FormState): ServiceCatalogItemInput {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    category: form.category,
    default_price: form.default_price.trim(),
    currency: form.currency.trim().toUpperCase(),
    recurrence: form.recurrence,
    cancellation_fee: form.cancellation_fee.trim() || "0",
    contract_template_id: form.contract_template_id
      ? Number(form.contract_template_id)
      : null,
    is_active: form.is_active,
  };
}

export type OverrideRow = {
  org: DistributorOption;
  price: string;
  currency: string;
};

export function editableModules(modules: PlatformModule[]) {
  return modules.filter((m) => m.level !== "core");
}

export function upsertOverrideRow(rows: OverrideRow[], row: OverrideRow) {
  return [...rows.filter((x) => x.org.uuid !== row.org.uuid), row];
}

export function deleteOverrideRow(rows: OverrideRow[], orgUuid: string) {
  return rows.filter((x) => x.org.uuid !== orgUuid);
}

export function overrideRowFromLookup(
  org: DistributorOption,
  override: ServiceCatalogOverride,
) {
  return {
    org,
    price: override.price,
    currency: override.currency,
  };
}

export function showsModulePicker(category: ServiceCatalogCategory) {
  return category === "module_bundle";
}

export function ServiceCatalogPage({
  mode,
  slug,
}: {
  mode: Mode;
  slug?: string;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const platform = mode === "platform";
  const [editing, setEditing] = useState<FormState | null>(null);
  const [overrideFor, setOverrideFor] = useState<ServiceCatalogItem | null>(
    null,
  );
  const list = useQuery({
    queryKey: platform
      ? serviceCatalogKeys.platform()
      : serviceCatalogKeys.visible(),
    queryFn: () =>
      platform
        ? serviceCatalogService.listPlatform()
        : serviceCatalogService.listVisible(),
  });
  const modules = useQuery({
    queryKey: ["service-catalog", "platform-modules"],
    queryFn: () => modulesService.platformList(),
    enabled: platform,
    staleTime: 60_000,
  });
  const templates = useQuery({
    queryKey: serviceCatalogKeys.templates(),
    queryFn: () => serviceCatalogService.listContractTemplates(),
    enabled: platform,
    retry: false,
    staleTime: 60_000,
  });

  const contractTemplates = useMemo(
    () => (templates.data?.items ?? []).filter((row) => row.id),
    [templates.data],
  );
  const rows = list.data?.items ?? [];

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: platform
        ? serviceCatalogKeys.platform()
        : serviceCatalogKeys.visible(),
    });
  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("catalog.toast.failed"),
    );

  const save = useMutation({
    mutationFn: async (form: FormState) => {
      const body = inputFromForm(form);
      const item = form.uuid
        ? await serviceCatalogService.patch(form.uuid, body)
        : await serviceCatalogService.create(body);
      if (body.category === "module_bundle") {
        return serviceCatalogService.setModules(item.uuid, form.modules);
      }
      return item;
    },
    onSuccess: async () => {
      await invalidate();
      setEditing(null);
      appToast.success(t("catalog.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => serviceCatalogService.delete(uuid),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("catalog.toast.deleted"));
    },
    onError,
  });

  return (
    <EntityPage
      title={t("catalog.services.title")}
      description={t(
        platform
          ? "catalog.services.description_platform"
          : "catalog.services.description_tenant",
      )}
      permission={
        platform
          ? permissions.serviceCatalog.manage
          : permissions.serviceCatalog.read
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.services.forbidden")}
        />
      }
      breadcrumbs={[
        {
          label: t("layout.breadcrumb_home"),
          href: platform
            ? routes.platform.home
            : routes.tenant.home(slug ?? ""),
        },
        { label: t("catalog.services.title") },
      ]}
      actions={
        platform ? (
          <Button type="button" onClick={() => setEditing(emptyForm)}>
            <Plus className="size-4" />
            {t("catalog.services.create")}
          </Button>
        ) : null
      }
    >
      {list.isLoading ? <Loading label={t("common.loading")} /> : null}
      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("catalog.services.load_failed")}
          retryLabel={t("common.retry")}
          onRetry={() => void list.refetch()}
        />
      ) : null}

      {!list.isLoading && !rows.length ? (
        <div className="text-muted-foreground rounded-lg border p-6 text-sm">
          {t("catalog.services.empty")}
        </div>
      ) : null}

      {rows.length ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-start font-medium">
                  {t("catalog.fields.name")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("catalog.fields.category")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("catalog.services.recurrence")}
                </th>
                <th className="px-3 py-2 text-end font-medium">
                  {t("catalog.distributor_prices.price")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("catalog.fields.active")}
                </th>
                {platform ? <th className="px-3 py-2" /> : null}
              </tr>
            </thead>
            <tbody>
              {rows.map((item) => {
                const price = platform
                  ? item.default_price
                  : item.effective_price?.amount;
                const currency = platform
                  ? item.currency
                  : item.effective_price?.currency || item.currency;
                return (
                  <tr key={item.uuid} className="border-t align-top">
                    <td className="px-3 py-2">
                      <div className="font-medium">{item.name}</div>
                      {item.description ? (
                        <div className="text-muted-foreground max-w-xl">
                          {item.description}
                        </div>
                      ) : null}
                      {item.modules?.length ? (
                        <div className="mt-2 flex flex-wrap gap-1">
                          {item.modules.map((key) => (
                            <Badge key={key} variant="outline">
                              {moduleName(t, key)}
                            </Badge>
                          ))}
                        </div>
                      ) : null}
                    </td>
                    <td className="px-3 py-2">
                      {t(`catalog.services.categories.${item.category}`)}
                    </td>
                    <td className="px-3 py-2">
                      {t(`catalog.services.recurrences.${item.recurrence}`)}
                    </td>
                    <td
                      className="px-3 py-2 text-end tabular-nums"
                      data-testid={`service-price-${item.uuid}`}
                    >
                      {price
                        ? format.currency(moneyValue(price), currency)
                        : t("catalog.services.price_masked")}
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant={item.is_active ? "success" : "outline"}>
                        {t(
                          item.is_active
                            ? "catalog.status.active"
                            : "catalog.status.inactive",
                        )}
                      </Badge>
                    </td>
                    {platform ? (
                      <td className="px-3 py-2 text-end whitespace-nowrap">
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          aria-label={t("common.edit")}
                          onClick={() => setEditing(formFromItem(item))}
                        >
                          <Pencil className="size-4" />
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={() => setOverrideFor(item)}
                        >
                          {t("catalog.services.overrides.tab")}
                        </Button>
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          className="text-destructive"
                          aria-label={t("common.delete")}
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(item.uuid)}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </td>
                    ) : null}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      {platform ? (
        <>
          <ServiceItemDialog
            form={editing}
            modules={editableModules(modules.data?.items ?? [])}
            templates={contractTemplates}
            pending={save.isPending}
            onChange={setEditing}
            onSubmit={(form) => save.mutate(form)}
          />
          <OverridesDialog
            key={overrideFor?.uuid ?? "closed"}
            item={overrideFor}
            onOpenChange={(open) => {
              if (!open) setOverrideFor(null);
            }}
          />
        </>
      ) : null}
    </EntityPage>
  );
}

export function ServiceItemDialog({
  form,
  modules,
  templates,
  pending,
  onChange,
  onSubmit,
}: {
  form: FormState | null;
  modules: PlatformModule[];
  templates: ContractTemplate[];
  pending: boolean;
  onChange: (form: FormState | null) => void;
  onSubmit: (form: FormState) => void;
}) {
  const { t } = useLocale();
  const update = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    if (!form) return;
    onChange({ ...form, [key]: value });
  };
  const moduleBundle = form ? showsModulePicker(form.category) : false;

  return (
    <Dialog
      open={Boolean(form)}
      onOpenChange={(open) => !open && onChange(null)}
    >
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {t(
              form?.uuid
                ? "catalog.services.edit_title"
                : "catalog.services.create_title",
            )}
          </DialogTitle>
        </DialogHeader>
        {form ? (
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              onSubmit(form);
            }}
          >
            <div className="grid gap-2 md:grid-cols-2">
              <Field label={t("catalog.fields.name")}>
                <Input
                  value={form.name}
                  required
                  onChange={(event) => update("name", event.target.value)}
                />
              </Field>
              <Field label={t("catalog.fields.category")}>
                <Select
                  value={form.category}
                  onValueChange={(v) =>
                    update("category", v as ServiceCatalogCategory)
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SERVICE_CATALOG_CATEGORIES.map((category) => (
                      <SelectItem key={category} value={category}>
                        {t(`catalog.services.categories.${category}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label={t("catalog.services.default_price")}>
                <Input
                  value={form.default_price}
                  required
                  inputMode="decimal"
                  onChange={(event) =>
                    update("default_price", event.target.value)
                  }
                />
              </Field>
              <Field label={t("catalog.prices.currency")}>
                <Input
                  value={form.currency}
                  required
                  maxLength={3}
                  onChange={(event) =>
                    update("currency", event.target.value.toUpperCase())
                  }
                />
              </Field>
              <Field label={t("catalog.services.recurrence")}>
                <Select
                  value={form.recurrence}
                  onValueChange={(v) =>
                    update("recurrence", v as ServiceCatalogRecurrence)
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SERVICE_CATALOG_RECURRENCES.map((recurrence) => (
                      <SelectItem key={recurrence} value={recurrence}>
                        {t(`catalog.services.recurrences.${recurrence}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label={t("catalog.services.cancellation_fee")}>
                <Input
                  value={form.cancellation_fee}
                  inputMode="decimal"
                  onChange={(event) =>
                    update("cancellation_fee", event.target.value)
                  }
                />
              </Field>
            </div>
            <Field label={t("catalog.fields.description_md")}>
              <Textarea
                value={form.description}
                onChange={(event) => update("description", event.target.value)}
              />
            </Field>
            {templates.length ? (
              <Field label={t("catalog.services.contract_template")}>
                <Select
                  value={form.contract_template_id || "none"}
                  onValueChange={(v) =>
                    update("contract_template_id", v === "none" ? "" : v)
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">
                      {t("catalog.services.no_contract_template")}
                    </SelectItem>
                    {templates.map((template) => (
                      <SelectItem
                        key={template.uuid}
                        value={String(template.id)}
                      >
                        {template.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            ) : null}
            <label className="flex items-center gap-2 text-sm">
              <Switch
                checked={form.is_active}
                onCheckedChange={(value) => update("is_active", value === true)}
              />
              {t("catalog.fields.active")}
            </label>
            {moduleBundle ? (
              <div
                className="rounded-lg border p-3"
                data-testid="module-picker"
              >
                <div className="mb-2 text-sm font-medium">
                  {t("catalog.services.modules")}
                </div>
                <div className="grid gap-2 md:grid-cols-2">
                  {modules.map((m) => {
                    const checked = form.modules.includes(m.key);
                    return (
                      <label
                        key={m.key}
                        className="flex items-center gap-2 rounded-md border p-2 text-sm"
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(value) => {
                            const next = value
                              ? [...form.modules, m.key]
                              : form.modules.filter((key) => key !== m.key);
                            update("modules", next);
                          }}
                        />
                        <span className="flex-1">
                          {moduleName(t, m.key)}
                          <span className="text-muted-foreground ms-2">
                            {moduleLevelLabel(t, m.level)}
                          </span>
                        </span>
                      </label>
                    );
                  })}
                </div>
              </div>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onChange(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={pending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function OverridesDialog({
  item,
  onOpenChange,
}: {
  item: ServiceCatalogItem | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [orgUuid, setOrgUuid] = useState("");
  const [price, setPrice] = useState("");
  const [currency, setCurrency] = useState(item?.currency ?? "");
  const [overrides, setOverrides] = useState<OverrideRow[]>([]);
  const distributors = useQuery({
    queryKey: serviceCatalogKeys.distributors(),
    queryFn: () => serviceCatalogService.listDistributors(),
    enabled: Boolean(item),
    staleTime: 60_000,
  });
  const selected = distributors.data?.find((d) => d.uuid === orgUuid);
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: serviceCatalogKeys.platform() });
  const onError = useCallback(
    (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("catalog.toast.failed"),
      );
    },
    [t],
  );

  useEffect(() => {
    if (!item || !orgUuid || !selected) return;
    let alive = true;
    void serviceCatalogService
      .getOverride(item.uuid, orgUuid)
      .then((saved) => {
        if (!alive) return;
        setPrice(saved.price);
        setCurrency(saved.currency);
        setOverrides((list) =>
          upsertOverrideRow(list, overrideRowFromLookup(selected, saved)),
        );
      })
      .catch((error: unknown) => {
        if (!alive) return;
        if (isApiError(error) && error.isNotFound) {
          setPrice("");
          setCurrency(item.currency);
          setOverrides((list) => deleteOverrideRow(list, orgUuid));
          return;
        }
        onError(error);
      });
    return () => {
      alive = false;
    };
  }, [item, onError, orgUuid, selected]);

  const put = useMutation({
    mutationFn: async () => {
      if (!item || !selected) throw new Error("missing override target");
      const saved = await serviceCatalogService.putOverride(
        item.uuid,
        selected.uuid,
        { price, currency: currency.toUpperCase() },
      );
      return overrideRowFromLookup(selected, saved);
    },
    onSuccess: async (row) => {
      setOverrides((list) => upsertOverrideRow(list, row));
      setPrice("");
      setCurrency(item?.currency ?? "");
      await invalidate();
      appToast.success(t("catalog.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: async (org: DistributorOption) => {
      if (!item) throw new Error("missing item");
      await serviceCatalogService.deleteOverride(item.uuid, org.uuid);
      return org.uuid;
    },
    onSuccess: async (uuid) => {
      setOverrides((list) => deleteOverrideRow(list, uuid));
      await invalidate();
      appToast.success(t("catalog.toast.deleted"));
    },
    onError,
  });

  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("catalog.services.overrides.title")}</DialogTitle>
        </DialogHeader>
        <Tabs defaultValue="edit">
          <TabsList>
            <TabsTrigger value="edit">
              {t("catalog.services.overrides.tab")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="edit" className="space-y-4">
            <div className="grid gap-3 md:grid-cols-[1fr_140px_100px_auto]">
              <Field label={t("catalog.distributor_prices.distributor")}>
                <Select value={orgUuid} onValueChange={setOrgUuid}>
                  <SelectTrigger>
                    <SelectValue
                      placeholder={t("catalog.services.overrides.pick")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {(distributors.data ?? []).map((d) => (
                      <SelectItem key={d.uuid} value={d.uuid}>
                        {d.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label={t("catalog.distributor_prices.price")}>
                <Input
                  value={price}
                  inputMode="decimal"
                  onChange={(event) => setPrice(event.target.value)}
                />
              </Field>
              <Field label={t("catalog.prices.currency")}>
                <Input
                  value={currency}
                  maxLength={3}
                  onChange={(event) =>
                    setCurrency(event.target.value.toUpperCase())
                  }
                />
              </Field>
              <div className="flex items-end">
                <Button
                  type="button"
                  disabled={!orgUuid || !price || put.isPending}
                  onClick={() => put.mutate()}
                >
                  {t("common.save")}
                </Button>
              </div>
            </div>
            <div className="rounded-lg border">
              {overrides.length ? (
                <table className="w-full text-sm">
                  <tbody>
                    {overrides.map((row) => (
                      <tr
                        key={row.org.uuid}
                        className="border-t first:border-0"
                      >
                        <td className="px-3 py-2">{row.org.name}</td>
                        <td className="px-3 py-2 text-end tabular-nums">
                          {format.currency(moneyValue(row.price), row.currency)}
                        </td>
                        <td className="px-3 py-2 text-end">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            aria-label={t("common.delete")}
                            disabled={remove.isPending}
                            onClick={() => remove.mutate(row.org)}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="text-muted-foreground p-3 text-sm">
                  {t("catalog.services.overrides.empty")}
                </p>
              )}
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}
