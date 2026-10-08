"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Info, ScanBarcode, Search, Trash2 } from "lucide-react";
import { useId, useState, type FormEvent } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import {
  CertificateWarningBand,
  blockingCertificateWarnings,
  completionBlockedReason,
} from "@/features/certificates/components/certificate-status";
import { isKnownPart, partsForItem } from "@/features/services/lib/car-parts";
import {
  normalizeMeters,
  stockErrorKey,
  unitMode,
  validateMeters,
  validateQuantity,
} from "@/features/services/lib/stock";
import {
  serviceWizardKeys,
  serviceWizardService,
  type Service,
  type ServiceItem,
  type ServiceItemInput,
  type ServiceStockUnit,
} from "@/features/services/services/service-wizard.service";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export type StockStepProps = {
  service: Service;
  /** Parts picked in step 2; each item gets those of its category. */
  selectedParts: readonly string[];
  onChanged: (service: Service) => void;
  onBack: () => void;
  onCompleted: (service: Service) => void;
  showCertificateWarnings?: boolean;
};

function useErrorText() {
  const { t } = useLocale();
  return (error: unknown) =>
    t(stockErrorKey(isApiError(error) ? error.code : undefined));
}

function AddUnitForm({
  service,
  unit,
  selectedParts,
  onAdded,
  onCancel,
}: {
  service: Service;
  unit: ServiceStockUnit;
  selectedParts: readonly string[];
  onAdded: (service: Service) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const errorText = useErrorText();
  const id = useId();
  const mode = unitMode(unit);
  const [meters, setMeters] = useState("");
  const [wholeRoll, setWholeRoll] = useState(false);
  const [quantity, setQuantity] = useState("1");
  const [submitted, setSubmitted] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const metersError =
    mode === "roll" && !wholeRoll
      ? validateMeters(meters, unit.remaining_meters)
      : null;
  const quantityError =
    mode === "fixed" ? validateQuantity(quantity, unit.quantity_on_hand) : null;
  const parts = partsForItem(selectedParts, unit.product.available_parts);
  // Exceeding the stock is shown as soon as it is typed.
  const showMeters =
    metersError && (submitted || metersError === "exceeds")
      ? metersError
      : null;
  const showQuantity =
    quantityError && (submitted || quantityError === "exceeds")
      ? quantityError
      : null;

  const add = useMutation({
    mutationFn: () => {
      const body: ServiceItemInput = {
        barcode: unit.barcode,
        product_uuid: unit.product.uuid,
        kind: mode === "roll" && !wholeRoll ? "partial" : "full",
        applied_parts: parts,
      };
      if (mode === "roll" && !wholeRoll) {
        body.meters = Number(normalizeMeters(meters));
      }
      if (mode === "fixed") body.quantity = Number(quantity);
      return serviceWizardService.addItem(service.uuid, body);
    },
    onSuccess: (saved) => {
      appToast.success(t("services.stock.added"));
      onAdded(saved);
    },
    onError: (error: unknown) => {
      if (isApiError(error)) {
        const fields = error.fieldErrors();
        if (fields.meters) {
          setServerError(t("services.stock.meters_errors.server"));
          return;
        }
        if (fields.quantity) {
          setServerError(t("services.stock.quantity_errors.server"));
          return;
        }
      }
      setServerError(errorText(error));
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    setServerError(null);
    if (metersError || quantityError) return;
    add.mutate();
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="bg-muted/30 space-y-4 rounded-lg border p-4"
      data-testid="add-unit-form"
      aria-labelledby={`${id}-title`}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <p id={`${id}-title`} className="font-medium">
            {unit.product.name}
          </p>
          <p className="text-muted-foreground font-mono text-xs" dir="ltr">
            {unit.barcode}
          </p>
        </div>
        <Badge variant="secondary">{t(`services.stock.mode.${mode}`)}</Badge>
      </div>

      {mode === "roll" ? (
        <div className="space-y-3">
          <p className="text-muted-foreground text-sm">
            {t("services.stock.remaining", {
              meters: unit.remaining_meters ?? "-",
            })}
          </p>
          <div className="flex items-center gap-2">
            <Checkbox
              id={`${id}-whole`}
              checked={wholeRoll}
              data-testid="whole-roll"
              onCheckedChange={(v) => setWholeRoll(v === true)}
            />
            <Label htmlFor={`${id}-whole`}>
              {t("services.stock.whole_roll")}
            </Label>
          </div>
          {!wholeRoll ? (
            <div className="space-y-2">
              <Label htmlFor={`${id}-meters`}>
                {t("services.stock.meters")}
              </Label>
              <Input
                id={`${id}-meters`}
                name="meters"
                inputMode="decimal"
                dir="ltr"
                autoComplete="off"
                className="max-w-40"
                value={meters}
                aria-invalid={showMeters ? true : undefined}
                aria-describedby={showMeters ? `${id}-meters-error` : undefined}
                onChange={(e) => {
                  setMeters(e.target.value);
                  setServerError(null);
                }}
              />
              {showMeters ? (
                <p
                  id={`${id}-meters-error`}
                  className="text-destructive text-sm"
                  role="alert"
                  data-testid="meters-error"
                >
                  {t(`services.stock.meters_errors.${showMeters}`, {
                    meters: unit.remaining_meters ?? "-",
                  })}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}

      {mode === "fixed" ? (
        <div className="space-y-2">
          <Label htmlFor={`${id}-qty`}>
            {t("services.stock.quantity", { count: unit.quantity_on_hand })}
          </Label>
          <Input
            id={`${id}-qty`}
            name="quantity"
            inputMode="numeric"
            dir="ltr"
            autoComplete="off"
            className="max-w-32"
            value={quantity}
            aria-invalid={showQuantity ? true : undefined}
            aria-describedby={showQuantity ? `${id}-qty-error` : undefined}
            onChange={(e) => {
              setQuantity(e.target.value);
              setServerError(null);
            }}
          />
          {showQuantity ? (
            <p
              id={`${id}-qty-error`}
              className="text-destructive text-sm"
              role="alert"
              data-testid="quantity-error"
            >
              {t(`services.stock.quantity_errors.${showQuantity}`, {
                count: unit.quantity_on_hand,
              })}
            </p>
          ) : null}
        </div>
      ) : null}

      <p className="text-muted-foreground text-sm" data-testid="item-parts">
        {parts.length > 0
          ? t("services.stock.parts_applied", { count: parts.length })
          : t("services.stock.parts_none")}
      </p>

      {serverError ? (
        <p
          className="text-destructive text-sm"
          role="alert"
          data-testid="add-error"
        >
          {serverError}
        </p>
      ) : null}

      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          data-testid="add-unit-submit"
          disabled={
            add.isPending ||
            showMeters === "exceeds" ||
            showQuantity === "exceeds"
          }
        >
          {t("services.stock.add")}
        </Button>
      </div>
    </form>
  );
}

function ItemRow({
  item,
  editable,
  onRemove,
  removing,
}: {
  item: ServiceItem;
  editable: boolean;
  onRemove: () => void;
  removing: boolean;
}) {
  const { t } = useLocale();
  const amount =
    item.kind === "partial" && item.meters
      ? t("services.stock.amount_meters", { meters: item.meters })
      : item.quantity
        ? t("services.stock.amount_pieces", { count: item.quantity })
        : t("services.stock.amount_whole");
  return (
    <li
      className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"
      data-testid="service-item"
      data-item={item.uuid}
    >
      <div className="min-w-0 space-y-1">
        <p className="font-medium">{item.product.name}</p>
        <p className="text-muted-foreground text-xs">
          <span className="font-mono" dir="ltr">
            {item.barcode}
          </span>
          <span className="ms-2">{amount}</span>
        </p>
        {item.applied_parts.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {item.applied_parts.map((p) => (
              <Badge key={p} variant="outline" className="text-xs">
                {isKnownPart(p) ? t(`services.parts.names.${p}`) : p}
              </Badge>
            ))}
          </div>
        ) : null}
      </div>
      {editable ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          disabled={removing}
          aria-label={t("services.stock.remove", { name: item.product.name })}
          data-testid="remove-item"
          onClick={onRemove}
        >
          <Trash2 className="size-4" />
        </Button>
      ) : null}
    </li>
  );
}

/**
 * Step 4: products and stock (TEC-182). Units of the service organization
 * come from the stock picker (TEC-180): a scanner (or typed barcode) plus
 * Enter looks the unit up exactly, the search lists by product name, SKU or
 * barcode. A roll takes meters (at most what is left) or the whole roll, a
 * fixed barcode pieces. The last step also completes the service when the
 * caller holds services.complete (stock is consumed by the backend).
 */
export function StockStep({
  service,
  selectedParts,
  onChanged,
  onBack,
  onCompleted,
  showCertificateWarnings = true,
}: StockStepProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const errorText = useErrorText();
  const scanId = useId();
  const searchId = useId();
  const [barcode, setBarcode] = useState("");
  const [scanError, setScanError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [pending, setPending] = useState<ServiceStockUnit | null>(null);
  const q = useDebounce(search.trim(), 300);
  const editable = service.items_editable;
  const items = service.items ?? [];

  const stock = useQuery({
    queryKey: serviceWizardKeys.stock(service.uuid, { q }),
    queryFn: () =>
      serviceWizardService.listStockUnits(service.uuid, q ? { q } : {}),
    enabled: editable,
  });

  const refreshStock = () =>
    queryClient.invalidateQueries({
      queryKey: ["service-wizard", "stock", service.uuid],
    });

  const scan = useMutation({
    mutationFn: (code: string) =>
      serviceWizardService.listStockUnits(service.uuid, { barcode: code }),
    onSuccess: (res) => {
      const unit = res.items[0];
      if (!unit) {
        setScanError(t("services.stock.scan_not_found"));
        return;
      }
      setBarcode("");
      setPending(unit);
    },
    onError: (error: unknown) => setScanError(errorText(error)),
  });

  const remove = useMutation({
    mutationFn: (item: string) =>
      serviceWizardService.removeItem(service.uuid, item),
    onSuccess: (saved) => {
      appToast.success(t("services.stock.removed"));
      onChanged(saved);
      void refreshStock();
    },
    onError: (error: unknown) => appToast.error(errorText(error)),
  });

  const canComplete =
    can(permissions.services.complete) &&
    service.available_transitions.includes("completed") &&
    (!showCertificateWarnings ||
      blockingCertificateWarnings(service).length === 0);
  const certificateBlock = showCertificateWarnings
    ? completionBlockedReason(service)
    : null;
  const complete = useMutation({
    mutationFn: () =>
      serviceWizardService.transition(service.uuid, "completed"),
    onSuccess: (saved) => {
      appToast.success(t("services.complete.done", { no: saved.service_no }));
      onCompleted(saved);
    },
    onError: (error: unknown) => appToast.error(errorText(error)),
  });

  const onScan = (e: FormEvent) => {
    e.preventDefault();
    const code = barcode.trim();
    setScanError(null);
    if (code === "") return;
    scan.mutate(code);
  };

  const units = stock.data?.items ?? [];

  return (
    <div className="space-y-6" data-testid="stock-step">
      {editable ? (
        <div className="grid gap-4 md:grid-cols-2">
          <form onSubmit={onScan} className="space-y-2" data-testid="scan-form">
            <Label htmlFor={scanId}>{t("services.stock.scan_label")}</Label>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <ScanBarcode className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
                <Input
                  id={scanId}
                  name="barcode"
                  dir="ltr"
                  autoFocus
                  autoComplete="off"
                  spellCheck={false}
                  maxLength={64}
                  className="ps-8 font-mono"
                  value={barcode}
                  aria-describedby={`${scanId}-hint`}
                  onChange={(e) => {
                    setBarcode(e.target.value);
                    setScanError(null);
                  }}
                />
              </div>
              <Button type="submit" variant="outline" disabled={scan.isPending}>
                {t("services.stock.scan_find")}
              </Button>
            </div>
            <p id={`${scanId}-hint`} className="text-muted-foreground text-xs">
              {t("services.stock.scan_hint")}
            </p>
            {scanError ? (
              <p
                className="text-destructive text-sm"
                role="alert"
                data-testid="scan-error"
              >
                {scanError}
              </p>
            ) : null}
          </form>
          <div className="space-y-2">
            <Label htmlFor={searchId}>{t("services.stock.search_label")}</Label>
            <div className="relative">
              <Search className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
              <Input
                id={searchId}
                name="q"
                type="search"
                autoComplete="off"
                maxLength={100}
                className="ps-8"
                placeholder={t("services.stock.search_placeholder")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
          </div>
        </div>
      ) : (
        <Alert>
          <Info />
          <AlertDescription>{t("services.stock.locked")}</AlertDescription>
        </Alert>
      )}

      {pending ? (
        <AddUnitForm
          key={pending.uuid}
          service={service}
          unit={pending}
          selectedParts={selectedParts}
          onCancel={() => setPending(null)}
          onAdded={(saved) => {
            setPending(null);
            onChanged(saved);
            void refreshStock();
          }}
        />
      ) : null}

      {editable ? (
        <section className="space-y-2" aria-labelledby={`${searchId}-units`}>
          <h3 id={`${searchId}-units`} className="text-sm font-semibold">
            {t("services.stock.units_title")}
          </h3>
          {stock.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("services.wizard.loading")}
            </p>
          ) : stock.isError ? (
            <p className="text-destructive text-sm" role="alert">
              {t("services.stock.load_failed")}
            </p>
          ) : units.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="units-empty"
            >
              {t("services.stock.units_empty")}
            </p>
          ) : (
            <ul
              className="divide-y rounded-lg border"
              data-testid="stock-units"
            >
              {units.map((u) => {
                const mode = unitMode(u);
                return (
                  <li
                    key={u.uuid}
                    className="flex flex-wrap items-center justify-between gap-2 p-3"
                    data-unit={u.barcode}
                  >
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{u.product.name}</p>
                      <p className="text-muted-foreground text-xs">
                        <span className="font-mono" dir="ltr">
                          {u.barcode}
                        </span>
                        <span className="ms-2">
                          {mode === "roll"
                            ? t("services.stock.remaining", {
                                meters: u.remaining_meters ?? "-",
                              })
                            : mode === "fixed"
                              ? t("services.stock.on_hand", {
                                  count: u.quantity_on_hand,
                                })
                              : t("services.stock.mode.piece")}
                        </span>
                      </p>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      data-testid="pick-unit"
                      onClick={() => setPending(u)}
                    >
                      {t("services.stock.pick")}
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      ) : null}

      <section className="space-y-2" aria-labelledby={`${scanId}-items`}>
        <h3 id={`${scanId}-items`} className="text-sm font-semibold">
          {t("services.stock.items_title", { count: items.length })}
        </h3>
        {items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="items-empty"
          >
            {t("services.stock.items_empty")}
          </p>
        ) : (
          <ul className="space-y-2">
            {items.map((item) => (
              <ItemRow
                key={item.uuid}
                item={item}
                editable={editable}
                removing={remove.isPending}
                onRemove={() => remove.mutate(item.uuid)}
              />
            ))}
          </ul>
        )}
      </section>

      {!can(permissions.services.complete) ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="complete-hint"
        >
          {t("services.complete.no_permission")}
        </p>
      ) : null}
      <CertificateWarningBand
        service={service}
        compact
        enabled={showCertificateWarnings}
      />
      {certificateBlock ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="complete-hint"
        >
          {t(certificateBlock)}
        </p>
      ) : null}

      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        {can(permissions.services.complete) ? (
          <Button
            type="button"
            data-testid="complete-service"
            disabled={!canComplete || items.length === 0 || complete.isPending}
            onClick={() => complete.mutate()}
          >
            <CheckCircle2 className="size-4" />
            {t("services.complete.button")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
