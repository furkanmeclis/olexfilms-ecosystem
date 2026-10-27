"use client";

import { useQuery } from "@tanstack/react-query";

import { FormFieldShell } from "@/components/forms/form-field";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

const ORGANIZATION_ROLE_TYPES = new Set(["center", "distributor", "dealer"]);

type RoleMultiSelectProps = {
  value: string[];
  onChange: (next: string[]) => void;
  error?: string;
  className?: string;
};

export function RoleMultiSelect({
  value,
  onChange,
  error,
  className,
}: RoleMultiSelectProps) {
  const { t } = useLocale();
  const { data, isLoading } = useQuery({
    queryKey: ["platform", "roles", "picker"],
    queryFn: () => rolesService.list({ limit: 100, offset: 0 }),
  });

  const selected = new Set(value);
  // TEC-1026: organization roles (center / distributor / dealer) are granted
  // through a membership (organization detail → add member), never globally;
  // the API rejects them here, so they are not offered.
  const roles = (data?.items ?? []).filter(
    (role) => !ORGANIZATION_ROLE_TYPES.has(role.org_type ?? ""),
  );

  const toggle = (uuid: string, checked: boolean) => {
    if (checked) {
      onChange([...value, uuid]);
      return;
    }
    onChange(value.filter((id) => id !== uuid));
  };

  return (
    <FormFieldShell
      name="role_uuids"
      label={t("users.fields.roles")}
      description={t("users.fields.roles_hint")}
      error={error}
      className={className}
    >
      <div className="space-y-2 rounded-md border p-3">
        {isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : null}
        {roles.map((role) => (
          <div key={role.uuid} className="flex items-start gap-2">
            <Checkbox
              id={`role-${role.uuid}`}
              checked={selected.has(role.uuid)}
              onCheckedChange={(checked) => toggle(role.uuid, checked === true)}
            />
            <Label
              htmlFor={`role-${role.uuid}`}
              className="cursor-pointer leading-snug font-normal"
            >
              <span className="font-medium">{role.name}</span>
              <span className="text-muted-foreground ms-2 font-mono text-xs">
                {role.slug}
              </span>
            </Label>
          </div>
        ))}
        {!isLoading && roles.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("users.roles_empty")}
          </p>
        ) : null}
      </div>
    </FormFieldShell>
  );
}
