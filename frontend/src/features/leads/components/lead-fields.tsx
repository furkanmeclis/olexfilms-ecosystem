"use client";

import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  LEAD_SOURCES,
  LEAD_TARGET_TYPES,
  LEAD_TEMPERATURES,
  type LeadFormValues,
} from "@/features/leads/lib/leads";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const leadInputClass =
  "border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";

export function LeadFields({
  values,
  errors,
  disabled,
  onChange,
}: {
  values: LeadFormValues;
  errors?: Record<string, string>;
  disabled?: boolean;
  onChange: (patch: Partial<LeadFormValues>) => void;
}) {
  const { t } = useLocale();
  const candidate = values.target_type !== "customer";
  const setTarget = (target_type: LeadFormValues["target_type"]) => {
    onChange({
      target_type,
      customer_user_id: "",
      vehicle_id: "",
      candidate_company_name: "",
      candidate_contact_name: "",
      country_id: "",
      province_id: "",
      district_id: "",
    });
  };

  const field = (
    name: keyof LeadFormValues,
    label: string,
    type = "text",
    extra?: { dir?: "ltr"; placeholder?: string },
  ) => (
    <div className="space-y-1.5">
      <Label htmlFor={`lead-${name}`}>{label}</Label>
      <input
        id={`lead-${name}`}
        data-testid={`lead-${name}`}
        className={cn(leadInputClass, errors?.[name] && "border-destructive")}
        type={type}
        dir={extra?.dir}
        placeholder={extra?.placeholder}
        value={values[name] as string}
        disabled={disabled}
        onChange={(e) => onChange({ [name]: e.target.value })}
      />
      {errors?.[name] ? (
        <p className="text-destructive text-xs" data-error={name}>
          {errors[name]}
        </p>
      ) : null}
    </div>
  );

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="lead-target-type">{t("leads.form.target_type")}</Label>
        <select
          id="lead-target-type"
          data-testid="lead-target-type"
          className={leadInputClass}
          value={values.target_type}
          disabled={disabled}
          onChange={(e) =>
            setTarget(e.target.value as LeadFormValues["target_type"])
          }
        >
          {LEAD_TARGET_TYPES.map((x) => (
            <option key={x} value={x}>
              {t(`leads.target_type.${x}`)}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="lead-source">{t("leads.form.source")}</Label>
        <select
          id="lead-source"
          data-testid="lead-source"
          className={leadInputClass}
          value={values.source}
          disabled={disabled}
          onChange={(e) =>
            onChange({ source: e.target.value as LeadFormValues["source"] })
          }
        >
          {LEAD_SOURCES.map((x) => (
            <option key={x} value={x}>
              {t(`leads.source.${x}`)}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="lead-temperature">{t("leads.form.temperature")}</Label>
        <select
          id="lead-temperature"
          data-testid="lead-temperature"
          className={leadInputClass}
          value={values.temperature}
          disabled={disabled}
          onChange={(e) =>
            onChange({
              temperature: e.target.value as LeadFormValues["temperature"],
            })
          }
        >
          {LEAD_TEMPERATURES.map((x) => (
            <option key={x} value={x}>
              {t(`leads.temperature.${x}`)}
            </option>
          ))}
        </select>
      </div>
      {field(
        "follow_up_date",
        t("leads.form.follow_up_date"),
        "datetime-local",
      )}
      {field("assignee_user_id", t("leads.form.assignee_user_id"), "number")}

      {candidate ? (
        <>
          {field("candidate_company_name", t("leads.form.company"))}
          {field("candidate_contact_name", t("leads.form.contact"))}
          {field("candidate_phone_e164", t("leads.form.phone"), "tel", {
            dir: "ltr",
            placeholder: "+905551234567",
          })}
          {field("candidate_email", t("leads.form.email"), "email", {
            dir: "ltr",
          })}
          {field("country_id", t("leads.form.country_id"), "number")}
          {field("province_id", t("leads.form.province_id"), "number")}
          {field("district_id", t("leads.form.district_id"), "number")}
        </>
      ) : (
        <>
          {field(
            "customer_user_id",
            t("leads.form.customer_user_id"),
            "number",
          )}
          {field("vehicle_id", t("leads.form.vehicle_id"), "number")}
          {field("candidate_phone_e164", t("leads.form.phone"), "tel", {
            dir: "ltr",
            placeholder: "+905551234567",
          })}
        </>
      )}

      <div className="space-y-1.5 sm:col-span-2">
        <Label htmlFor="lead-notes">{t("leads.form.notes")}</Label>
        <Textarea
          id="lead-notes"
          data-testid="lead-notes"
          rows={4}
          value={values.notes}
          disabled={disabled}
          onChange={(e) => onChange({ notes: e.target.value })}
        />
      </div>
    </div>
  );
}
