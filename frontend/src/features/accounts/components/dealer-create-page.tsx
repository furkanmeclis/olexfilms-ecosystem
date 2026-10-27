"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import {
  accountsService,
  type TenantDealerResult,
} from "@/features/accounts/services/accounts.service";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { useLocale } from "@/providers/locale-provider";

type DealerValues = { name: string; city: string; phone: string };

const emptyDealer: DealerValues = { name: "", city: "", phone: "" };

/**
 * Opens a dealer directly under the active center (or distributor) with its
 * owner (organizations.write). The dealer starts active and can order at
 * once; the owner signs in on the phone with a WhatsApp code.
 */
export function DealerCreatePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [dealer, setDealer] = useState<DealerValues>(emptyDealer);
  const [owner, setOwner] = useState<AccountValues>(emptyAccount);
  const [touched, setTouched] = useState(false);
  const [created, setCreated] = useState<TenantDealerResult | null>(null);

  const create = useMutation({
    mutationFn: () =>
      accountsService.createDealer({
        name: dealer.name.trim(),
        ...(dealer.city.trim() ? { city: dealer.city.trim() } : {}),
        ...(dealer.phone.trim() ? { phone: dealer.phone.trim() } : {}),
        owner: accountBody(owner),
      }),
    onSuccess: (r) => {
      setCreated(r);
      // The dealer pickers (orders, network) list it from now on.
      void qc.invalidateQueries({ queryKey: ["orders"] });
      void qc.invalidateQueries({ queryKey: ["tenant", "organizations"] });
      toast.success(t("organizations.accounts.dealer_created"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("organizations.accounts.save_failed")),
      ),
  });

  const reset = () => {
    setDealer(emptyDealer);
    setOwner(emptyAccount);
    setTouched(false);
    setCreated(null);
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setTouched(true);
    if (dealer.name.trim() && accountValid(owner)) create.mutate();
  };

  return (
    <EntityPage
      title={t("organizations.accounts.dealer_title")}
      description={t("organizations.accounts.dealer_description")}
      permission={Permission.OrganizationsWrite}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.accounts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("organizations.accounts.dealer_title") },
      ]}
    >
      {created ? (
        <AccountCreated
          title={t("organizations.accounts.dealer_created")}
          lines={[
            created.organization.name,
            `${created.owner.name} · ${created.owner.phone}`,
            ...(created.owner.existing
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
                {t("organizations.accounts.section_dealer")}
              </CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="dealer-name">
                  {t("organizations.accounts.fields.dealer_name")}
                </Label>
                <Input
                  id="dealer-name"
                  value={dealer.name}
                  onChange={(e) =>
                    setDealer({ ...dealer, name: e.target.value })
                  }
                />
                {touched && !dealer.name.trim() ? (
                  <p className="text-destructive text-sm">
                    {t("organizations.accounts.errors.dealer_name_required")}
                  </p>
                ) : null}
              </div>
              <div className="space-y-2">
                <Label htmlFor="dealer-city">
                  {t("organizations.accounts.fields.city")}
                </Label>
                <Input
                  id="dealer-city"
                  value={dealer.city}
                  onChange={(e) =>
                    setDealer({ ...dealer, city: e.target.value })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="dealer-phone">
                  {t("organizations.accounts.fields.dealer_phone")}
                </Label>
                <Input
                  id="dealer-phone"
                  type="tel"
                  inputMode="tel"
                  value={dealer.phone}
                  onChange={(e) =>
                    setDealer({ ...dealer, phone: e.target.value })
                  }
                />
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("organizations.accounts.section_owner")}</CardTitle>
            </CardHeader>
            <CardContent>
              <AccountFields
                idPrefix="owner"
                value={owner}
                onChange={setOwner}
                touched={touched}
              />
            </CardContent>
          </Card>
          <div className="flex justify-end">
            <Button type="submit" disabled={create.isPending}>
              {t("organizations.accounts.dealer_submit")}
            </Button>
          </div>
        </form>
      )}
    </EntityPage>
  );
}
