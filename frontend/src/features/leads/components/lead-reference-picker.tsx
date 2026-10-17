"use client";

import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { customersService } from "@/features/customers/services/customers.service";
import { leadsService } from "@/features/leads/services/leads.service";
import {
  vehiclesService,
  type Vehicle,
} from "@/features/vehicles/services/vehicles.service";
import { useLocale } from "@/providers/locale-provider";

type Kind = "assignee" | "customer" | "vehicle";
const fieldNames = {
  assignee: "assignee_user_uuid",
  customer: "customer_uuid",
  vehicle: "vehicle_uuid",
} as const;
const vehicleOption = (v: Vehicle): ComboboxOption => ({
  value: v.uuid,
  label:
    [v.plate || v.vin, v.car_brand?.name, v.car_model?.name]
      .filter(Boolean)
      .join(" · ") || v.uuid,
});

/** Scoped lead references, sharing the searchable combobox used by other forms. */
export function LeadReferencePicker({
  kind,
  value,
  onValueChange,
  organizationUuid,
  customerUuid,
  disabled,
  error,
  id = `lead-${fieldNames[kind]}`,
  className,
}: {
  kind: Kind;
  value: string;
  onValueChange: (value: string) => void;
  organizationUuid?: string;
  customerUuid?: string;
  disabled?: boolean;
  error?: string;
  id?: string;
  className?: string;
}) {
  const { t } = useLocale();
  const [seen, setSeen] = useState<ComboboxOption[]>([]);
  const members = useQuery({
    queryKey: ["leads", "assignees", organizationUuid],
    queryFn: () => leadsService.assignees(organizationUuid),
    enabled: kind === "assignee",
    staleTime: 60_000,
  });
  const selected = useQuery({
    queryKey: ["leads", "reference", kind, value],
    queryFn: async (): Promise<ComboboxOption> => {
      if (kind === "customer") {
        const c = await customersService.get(value);
        return {
          value: c.uuid,
          label: [c.name, c.surname].filter(Boolean).join(" "),
          description: c.phone ?? c.email ?? undefined,
        };
      }
      return vehicleOption(await vehiclesService.get(value));
    },
    enabled:
      kind !== "assignee" &&
      Boolean(value) &&
      !seen.some((o) => o.value === value),
    staleTime: 60_000,
  });
  const loadOptions = useCallback(
    async (q: string) => {
      const query = { q: q.trim() || undefined, limit: 20, offset: 0 };
      const options =
        kind === "customer"
          ? (await customersService.list(query)).items.map(
              (c): ComboboxOption => ({
                value: c.uuid,
                label: [c.name, c.surname].filter(Boolean).join(" "),
                description: c.phone ?? c.email ?? undefined,
              }),
            )
          : (
              await vehiclesService.list({
                ...query,
                customer_uuid: customerUuid || undefined,
              })
            ).items.map(vehicleOption);
      setSeen((prev) => [
        ...new Map([...prev, ...options].map((o) => [o.value, o])).values(),
      ]);
      return options;
    },
    [kind, customerUuid],
  );
  const options =
    kind === "assignee"
      ? (members.data?.items ?? []).map((m) => ({
          value: m.uuid,
          label:
            [m.name, m.surname].filter(Boolean).join(" ") || m.email || m.uuid,
          description: m.email,
        }))
      : [...seen, ...(selected.data ? [selected.data] : [])];
  const lookup = kind === "assignee" ? members : selected;

  return (
    <div className={className ?? "space-y-1.5"}>
      <Label htmlFor={id}>{t(`leads.form.${kind}`)}</Label>
      <AsyncCombobox
        id={id}
        data-testid={id}
        value={value}
        onValueChange={onValueChange}
        options={options}
        loadOptions={kind === "assignee" ? undefined : loadOptions}
        placeholder={t(`leads.form.${kind}`)}
        disabled={disabled || (kind === "assignee" && members.isLoading)}
        aria-invalid={Boolean(error)}
        aria-describedby={error ? `${id}-error` : undefined}
        clearable
      />
      {error ? (
        <p
          id={`${id}-error`}
          className="text-destructive text-xs"
          data-error={fieldNames[kind]}
        >
          {error}
        </p>
      ) : null}
      {lookup.isError ? (
        <div role="alert" className="text-destructive text-xs">
          {t("common.error_generic")}
          <Button
            type="button"
            variant="link"
            size="sm"
            onClick={() => void lookup.refetch()}
          >
            {t("common.retry")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
