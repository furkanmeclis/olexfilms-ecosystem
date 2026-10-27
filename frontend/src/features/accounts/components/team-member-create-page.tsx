"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AccountCreated,
  AccountFields,
  accountBody,
  accountValid,
  emptyAccount,
  type AccountValues,
} from "@/features/accounts/components/account-fields";
import { isWarehouseRole, staffRoles } from "@/features/accounts/lib/roles";
import {
  accountsService,
  type TenantMember,
} from "@/features/accounts/services/accounts.service";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Adds a staff member to the active organization (members.write): the
 * phone is the mobile sign-in (WhatsApp code), the role one of the
 * organization's staff roles, and a warehouse role joins the picked
 * warehouses (the primary one preselected).
 */
export function TeamMemberCreatePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = usePermission();
  const org = useActiveOrganization(slug);
  const roles = staffRoles(org?.type);
  const [account, setAccount] = useState<AccountValues>(emptyAccount);
  const [role, setRole] = useState<string | null>(null);
  const [picked, setPicked] = useState<Set<string> | null>(null);
  const [touched, setTouched] = useState(false);
  const [created, setCreated] = useState<TenantMember | null>(null);
  const roleValue = role ?? roles[0] ?? "";
  const warehouseRole = isWarehouseRole(roleValue);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled: warehouseRole && access.can(Permission.WarehouseRead),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const active = useMemo(
    () => (warehouses.data?.items ?? []).filter((w) => w.active),
    [warehouses.data?.items],
  );
  // The primary warehouse is preselected until the user picks.
  const selected =
    picked ?? new Set(active.filter((w) => w.is_primary).map((w) => w.uuid));

  const create = useMutation({
    mutationFn: () =>
      accountsService.createMember({
        ...accountBody(account),
        ...(roleValue ? { role_slugs: [roleValue] } : {}),
        ...(warehouseRole && selected.size
          ? { warehouse_uuids: [...selected] }
          : {}),
      }),
    onSuccess: (m) => {
      setCreated(m);
      toast.success(t("organizations.accounts.member_created"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("organizations.accounts.save_failed")),
      ),
  });

  const reset = () => {
    setAccount(emptyAccount);
    setTouched(false);
    setCreated(null);
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setTouched(true);
    if (accountValid(account)) create.mutate();
  };

  return (
    <EntityPage
      title={t("organizations.accounts.member_title")}
      description={t("organizations.accounts.member_description")}
      permission={Permission.MembersWrite}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.accounts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("organizations.accounts.member_title") },
      ]}
    >
      {created ? (
        <AccountCreated
          title={t("organizations.accounts.member_created")}
          lines={[
            `${created.name} ${created.surname === "-" ? "" : created.surname}`.trim(),
            created.phone,
            created.role_slugs.map((r) => t(`roles.names.${r}`)).join(", "),
            ...(created.existing
              ? [t("organizations.accounts.existing_user")]
              : []),
          ]}
          onAnother={reset}
        />
      ) : (
        <form onSubmit={submit} className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>
                {t("organizations.accounts.section_person")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <AccountFields
                idPrefix="member"
                value={account}
                onChange={setAccount}
                touched={touched}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>
                {t("organizations.accounts.section_access")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="member-role">
                  {t("organizations.accounts.fields.role")}
                </Label>
                <Select value={roleValue} onValueChange={setRole}>
                  <SelectTrigger id="member-role" data-testid="member-role">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {roles.map((r) => (
                      <SelectItem key={r} value={r}>
                        {t(`roles.names.${r}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {warehouseRole && active.length > 0 ? (
                <fieldset className="space-y-2">
                  <legend className="text-sm font-medium">
                    {t("organizations.accounts.fields.warehouses")}
                  </legend>
                  {active.map((w) => (
                    <label
                      key={w.uuid}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Checkbox
                        checked={selected.has(w.uuid)}
                        onCheckedChange={(on) => {
                          const next = new Set(selected);
                          if (on) next.add(w.uuid);
                          else next.delete(w.uuid);
                          setPicked(next);
                        }}
                      />
                      {w.name}
                    </label>
                  ))}
                  <p className="text-muted-foreground text-sm">
                    {t("organizations.accounts.fields.warehouses_hint")}
                  </p>
                </fieldset>
              ) : null}
            </CardContent>
          </Card>
          <div className="flex justify-end">
            <Button type="submit" disabled={create.isPending}>
              {t("organizations.accounts.member_submit")}
            </Button>
          </div>
        </form>
      )}
    </EntityPage>
  );
}
