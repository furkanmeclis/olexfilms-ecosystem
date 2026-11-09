"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { OrganizationType } from "@/features/organizations/services/organizations.service";
import {
  roleDisplayDescription,
  roleDisplayName,
} from "@/features/roles/lib/role-display";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

type MemberRolesFormValues = { role_slugs?: string[] };

/**
 * TEC-1026: organization roles of the org type (center_accounting,
 * dealer_accounting, ...). Empty means the default role of the membership
 * kind (owner → center_owner / distributor_owner / dealer_owner, staff →
 * center_staff / distributor_staff / dealer_staff); a selection replaces it,
 * so an accounting-only account is one that picks only "accounting".
 * TEC-1036: shared by "Add staff" and "Edit roles" (the form's `role_slugs`).
 */
export function OrganizationMemberRolesField({
  organizationType,
}: {
  organizationType: OrganizationType;
}) {
  const { t } = useLocale();
  const form = useFormContext<MemberRolesFormValues>();
  const { data, isLoading } = useQuery({
    queryKey: ["platform", "roles", "picker", organizationType],
    queryFn: () =>
      rolesService.list({ limit: 100, offset: 0, is_system: "true" }),
  });
  const roles = useMemo(
    () =>
      (data?.items ?? []).filter((role) => role.org_type === organizationType),
    [data, organizationType],
  );
  const value = form.watch("role_slugs") ?? [];
  const selected = new Set(value);
  const toggle = (slug: string, checked: boolean) => {
    const next = checked
      ? [...value, slug]
      : value.filter((item) => item !== slug);
    form.setValue("role_slugs", next, { shouldDirty: true });
  };

  return (
    <FormFieldShell
      name="role_slugs"
      label={t("organizations.fields.member_roles")}
      description={t("organizations.fields.member_roles_hint")}
    >
      <div
        className="space-y-2 rounded-md border p-3"
        data-testid="member-role-slugs"
      >
        {isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : null}
        {roles.map((role) => {
          const description = roleDisplayDescription(role, t);
          return (
            <div key={role.slug} className="flex items-start gap-2">
              <Checkbox
                id={`member-role-${role.slug}`}
                checked={selected.has(role.slug)}
                onCheckedChange={(checked) =>
                  toggle(role.slug, checked === true)
                }
              />
              <Label
                htmlFor={`member-role-${role.slug}`}
                className="cursor-pointer flex-col items-start gap-0.5 leading-snug font-normal"
              >
                <span>
                  <span className="font-medium">
                    {roleDisplayName(role, t)}
                  </span>
                  <span className="text-muted-foreground ms-2 font-mono text-xs">
                    {role.slug}
                  </span>
                </span>
                {description ? (
                  <span className="text-muted-foreground text-xs">
                    {description}
                  </span>
                ) : null}
              </Label>
            </div>
          );
        })}
      </div>
    </FormFieldShell>
  );
}
