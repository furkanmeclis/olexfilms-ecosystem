"use client";

import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { DateTimePicker } from "@/components/ui/date-time-picker";
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
  targetTypes = LEAD_TARGET_TYPES,
  onChange,
}: {
  values: LeadFormValues;
  errors?: Record<string, string>;
  disabled?: boolean;
  /** Offered target types; the current value always stays selectable. */
  targetTypes?: readonly LeadFormValues["target_type"][];
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
        <Select
          value={values.target_type}
          disabled={disabled}
          onValueChange={(value) =>
            setTarget(value as LeadFormValues["target_type"])
          }
        >
          <SelectTrigger
            id="lead-target-type"
            data-testid="lead-target-type"
            className="h-10"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {LEAD_TARGET_TYPES.filter(
              (x) => x === values.target_type || targetTypes.includes(x),
            ).map((x) => (
              <SelectItem key={x} value={x}>
                {t(`leads.target_type.${x}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="lead-source">{t("leads.form.source")}</Label>
        <Select
          value={values.source}
          disabled={disabled}
          onValueChange={(value) =>
            onChange({ source: value as LeadFormValues["source"] })
          }
        >
          <SelectTrigger
            id="lead-source"
            data-testid="lead-source"
            className="h-10"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {LEAD_SOURCES.map((x) => (
              <SelectItem key={x} value={x}>
                {t(`leads.source.${x}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="lead-temperature">{t("leads.form.temperature")}</Label>
        <Select
          value={values.temperature}
          disabled={disabled}
          onValueChange={(value) =>
            onChange({ temperature: value as LeadFormValues["temperature"] })
          }
        >
          <SelectTrigger
            id="lead-temperature"
            data-testid="lead-temperature"
            className="h-10"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {LEAD_TEMPERATURES.map((x) => (
              <SelectItem key={x} value={x}>
                {t(`leads.temperature.${x}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="lead-follow_up_date">
          {t("leads.form.follow_up_date")}
        </Label>
        <DateTimePicker
          id="lead-follow_up_date"
          data-testid="lead-follow_up_date"
          value={values.follow_up_date}
          disabled={disabled}
          aria-invalid={errors?.follow_up_date ? true : undefined}
          onChange={(value) => onChange({ follow_up_date: value })}
        />
        {errors?.follow_up_date ? (
          <p className="text-destructive text-xs" data-error="follow_up_date">
            {errors.follow_up_date}
          </p>
        ) : null}
      </div>
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
