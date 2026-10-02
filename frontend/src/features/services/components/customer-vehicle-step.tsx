"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Search } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NewCustomerForm } from "@/features/services/components/new-customer-form";
import { NewVehicleForm } from "@/features/services/components/new-vehicle-form";
import type { ServiceWizardAccess } from "@/features/services/lib/access";
import { parseKm } from "@/features/services/lib/wizard";
import {
  serviceWizardKeys,
  serviceWizardService,
  type Service,
  type Vehicle,
} from "@/features/services/services/service-wizard.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type PickedCustomer = {
  uuid: string;
  name: string;
  surname: string;
  phone: string | null;
};

export type CustomerVehicleStepProps = {
  access: ServiceWizardAccess;
  /** The draft, once created: customer and vehicle are then fixed. */
  service?: Service | null;
  /** Called with the created or updated draft; the wizard moves on. */
  onDone: (service: Service) => void;
};

const customerLabel = (c: { name: string; surname: string }) =>
  [c.name, c.surname].filter(Boolean).join(" ");

/** A service needs the car brand and model on the vehicle (TEC-179). */
export const vehicleUsable = (v: Vehicle) =>
  Boolean(v.car_brand && v.car_model);

function VehicleSummary({
  brandUuid,
  brand,
  model,
  plate,
  year,
  vin,
}: {
  brandUuid?: string | null;
  brand?: string | null;
  model?: string | null;
  plate?: string | null;
  year?: number | null;
  vin?: string | null;
}) {
  const { t } = useLocale();
  return (
    <div className="flex min-w-0 items-center gap-3">
      <VehicleBrandLogo
        uuid={brandUuid}
        name={brand ?? undefined}
        height={36}
      />
      <div className="min-w-0 text-start">
        <p className="truncate font-medium">
          {[brand, model].filter(Boolean).join(" ") ||
            t("services.vehicle.no_model")}
          {year ? (
            <span className="text-muted-foreground ms-1 font-normal">
              ({year})
            </span>
          ) : null}
        </p>
        <p className="text-muted-foreground truncate text-sm" dir="ltr">
          {plate ?? "—"}
          {vin ? <span className="ms-2 font-mono text-xs">{vin}</span> : null}
        </p>
      </div>
    </div>
  );
}

/**
 * Step 1: pick or create the customer (customers in scope, TEC-160), pick
 * or create one of the customer's vehicles (car brand logo, TEC-150) and
 * open the draft service (POST /v1/services). After the draft exists the
 * customer and vehicle stay fixed (the service keeps a vehicle snapshot);
 * only km is editable here.
 */
export function CustomerVehicleStep({
  access,
  service,
  onDone,
}: CustomerVehicleStepProps) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const debounced = useDebounce(query.trim(), 300);
  const [customer, setCustomer] = useState<PickedCustomer | null>(null);
  const [vehicle, setVehicle] = useState<Vehicle | null>(null);
  const [newCustomer, setNewCustomer] = useState(false);
  const [newVehicle, setNewVehicle] = useState(false);
  const [km, setKm] = useState(
    service?.km !== null && service?.km !== undefined ? String(service.km) : "",
  );
  const [kmError, setKmError] = useState<string | null>(null);
  const locked = Boolean(service);

  const customers = useQuery({
    queryKey: serviceWizardKeys.customers(debounced),
    queryFn: () => serviceWizardService.listCustomers({ q: debounced }),
    enabled: !locked && !customer && debounced.length >= 2,
  });

  const vehicles = useQuery({
    queryKey: serviceWizardKeys.vehicles(customer?.uuid ?? ""),
    queryFn: () => serviceWizardService.listVehicles(customer?.uuid ?? ""),
    enabled: !locked && Boolean(customer),
  });

  const save = useMutation({
    mutationFn: (kmValue: number | null) => {
      if (service) {
        return serviceWizardService.updateService(service.uuid, {
          km: kmValue,
        });
      }
      return serviceWizardService.createService({
        customer_uuid: customer?.uuid ?? "",
        vehicle_uuid: vehicle?.uuid ?? "",
        // Answered in step 3, which also saves the VIN.
        has_measurement: false,
        ...(kmValue !== null ? { km: kmValue } : {}),
      });
    },
    onSuccess: (saved) => {
      appToast.success(
        service
          ? t("services.wizard.saved")
          : t("services.wizard.draft_created", { no: saved.service_no }),
      );
      onDone(saved);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("services.wizard.save_failed"),
      ),
  });

  const pickCustomer = (c: PickedCustomer) => {
    setCustomer(c);
    setVehicle(null);
    setNewCustomer(false);
    setNewVehicle(false);
  };

  const submit = () => {
    const parsed = parseKm(km);
    if (parsed === "invalid") {
      setKmError(t("services.wizard.km_invalid"));
      return;
    }
    setKmError(null);
    if (!service && (!customer || !vehicle)) return;
    save.mutate(parsed);
  };

  const kmField = (
    <div className="max-w-xs space-y-2">
      <Label htmlFor="service-km">{t("services.wizard.km")}</Label>
      <Input
        id="service-km"
        name="km"
        inputMode="numeric"
        dir="ltr"
        value={km}
        disabled={locked && !service?.editable}
        aria-invalid={kmError ? true : undefined}
        onChange={(e) => setKm(e.target.value)}
      />
      {kmError ? <p className="text-destructive text-xs">{kmError}</p> : null}
    </div>
  );

  if (service) {
    return (
      <div className="space-y-6" data-testid="customer-vehicle-step">
        <Alert>
          <AlertDescription>
            {t("services.wizard.locked_hint")}
          </AlertDescription>
        </Alert>
        <section className="space-y-2">
          <h3 className="text-sm font-medium">
            {t("services.customer.title")}
          </h3>
          <p data-testid="locked-customer">
            {customerLabel(service.customer)}
            {service.customer.phone ? (
              <span className="text-muted-foreground ms-2 text-sm" dir="ltr">
                {service.customer.phone}
              </span>
            ) : null}
          </p>
        </section>
        <section className="space-y-2">
          <h3 className="text-sm font-medium">{t("services.vehicle.title")}</h3>
          <VehicleSummary
            brandUuid={service.car_brand.uuid}
            brand={service.car_brand.name}
            model={service.car_model.name}
            plate={service.plate}
            year={service.model_year}
            vin={service.vin}
          />
        </section>
        {kmField}
        <div className="flex justify-end">
          <Button
            type="button"
            data-testid="step1-continue"
            disabled={save.isPending}
            onClick={submit}
          >
            {t("services.wizard.save_continue")}
          </Button>
        </div>
      </div>
    );
  }

  const vehicleItems = vehicles.data?.items ?? [];

  return (
    <div className="space-y-8" data-testid="customer-vehicle-step">
      {/* Customer */}
      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-medium">
            {t("services.customer.title")}
          </h3>
          {access.canCreateCustomer && !customer && !newCustomer ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              data-testid="new-customer-toggle"
              onClick={() => setNewCustomer(true)}
            >
              <Plus />
              {t("services.customer.new")}
            </Button>
          ) : null}
        </div>

        {customer ? (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
            <p data-testid="picked-customer">
              <span className="font-medium">{customerLabel(customer)}</span>
              {customer.phone ? (
                <span className="text-muted-foreground ms-2 text-sm" dir="ltr">
                  {customer.phone}
                </span>
              ) : null}
            </p>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setCustomer(null);
                setVehicle(null);
                setNewVehicle(false);
              }}
            >
              {t("services.wizard.change")}
            </Button>
          </div>
        ) : newCustomer ? (
          <NewCustomerForm
            onCancel={() => setNewCustomer(false)}
            onCreated={(c) => {
              void queryClient.invalidateQueries({
                queryKey: serviceWizardKeys.all,
              });
              pickCustomer({
                uuid: c.uuid,
                name: c.name,
                surname: c.surname,
                phone: c.phone,
              });
            }}
          />
        ) : (
          <div className="space-y-2">
            <div className="relative">
              <Search className="text-muted-foreground pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2" />
              <Input
                name="customer-search"
                className="ps-9"
                placeholder={t("services.customer.search_placeholder")}
                aria-label={t("services.customer.search_placeholder")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            {debounced.length < 2 ? (
              <p className="text-muted-foreground text-xs">
                {t("services.customer.search_hint")}
              </p>
            ) : customers.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("services.wizard.loading")}
              </p>
            ) : customers.isError ? (
              <p className="text-destructive text-sm">
                {t("services.customer.load_failed")}
              </p>
            ) : (customers.data?.items ?? []).length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("services.customer.none_found")}
              </p>
            ) : (
              <ul className="divide-y rounded-lg border" role="list">
                {(customers.data?.items ?? []).map((c) => (
                  <li key={c.uuid}>
                    <button
                      type="button"
                      data-testid="customer-option"
                      className="hover:bg-accent flex w-full items-center justify-between gap-3 px-3 py-2 text-start"
                      onClick={() =>
                        pickCustomer({
                          uuid: c.uuid,
                          name: c.name,
                          surname: c.surname,
                          phone: c.phone,
                        })
                      }
                    >
                      <span className="font-medium">{customerLabel(c)}</span>
                      {c.phone ? (
                        <span
                          className="text-muted-foreground text-sm"
                          dir="ltr"
                        >
                          {c.phone}
                        </span>
                      ) : null}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </section>

      {/* Vehicle */}
      {customer ? (
        <section className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("services.vehicle.title")}
            </h3>
            {access.canCreateVehicle && !newVehicle ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="new-vehicle-toggle"
                onClick={() => setNewVehicle(true)}
              >
                <Plus />
                {t("services.vehicle.new")}
              </Button>
            ) : null}
          </div>
          {newVehicle ? (
            <NewVehicleForm
              customerUuid={customer.uuid}
              onCancel={() => setNewVehicle(false)}
              onCreated={(v) => {
                void queryClient.invalidateQueries({
                  queryKey: serviceWizardKeys.vehicles(customer.uuid),
                });
                setNewVehicle(false);
                setVehicle(v);
              }}
            />
          ) : null}
          {vehicles.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("services.wizard.loading")}
            </p>
          ) : vehicles.isError ? (
            <p className="text-destructive text-sm">
              {t("services.vehicle.load_failed")}
            </p>
          ) : vehicleItems.length === 0 && !newVehicle ? (
            <p className="text-muted-foreground text-sm">
              {t("services.vehicle.none")}
            </p>
          ) : (
            <div
              className="grid gap-2 sm:grid-cols-2"
              role="radiogroup"
              aria-label={t("services.vehicle.title")}
            >
              {vehicleItems.map((v) => {
                const usable = vehicleUsable(v);
                const selected = vehicle?.uuid === v.uuid;
                return (
                  <button
                    key={v.uuid}
                    type="button"
                    role="radio"
                    aria-checked={selected}
                    disabled={!usable}
                    data-testid="vehicle-option"
                    className={cn(
                      "rounded-lg border p-3 text-start transition-colors disabled:cursor-not-allowed disabled:opacity-60",
                      selected
                        ? "border-primary ring-primary/30 ring-2"
                        : "hover:bg-accent",
                    )}
                    onClick={() => setVehicle(v)}
                  >
                    <VehicleSummary
                      brandUuid={v.car_brand?.uuid}
                      brand={v.car_brand?.name}
                      model={v.car_model?.name}
                      plate={v.plate}
                      year={v.model_year}
                      vin={v.vin}
                    />
                    {!usable ? (
                      <p className="text-muted-foreground mt-2 text-xs">
                        {t("services.vehicle.needs_model")}
                      </p>
                    ) : null}
                  </button>
                );
              })}
            </div>
          )}
        </section>
      ) : null}

      {customer && vehicle ? kmField : null}

      <div className="flex justify-end">
        <Button
          type="button"
          data-testid="step1-continue"
          disabled={!customer || !vehicle || save.isPending}
          onClick={submit}
        >
          {t("services.wizard.create_draft")}
        </Button>
      </div>
    </div>
  );
}
