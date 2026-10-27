"use client";

import { CheckCircle2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

export type AccountValues = {
  name: string;
  surname: string;
  phone: string;
  email: string;
};

export const emptyAccount: AccountValues = {
  name: "",
  surname: "",
  phone: "",
  email: "",
};

/** The request body of a person (empty e-mail and surname left out). */
export function accountBody(v: AccountValues) {
  return {
    name: v.name.trim(),
    phone: v.phone.trim(),
    ...(v.surname.trim() ? { surname: v.surname.trim() } : {}),
    ...(v.email.trim() ? { email: v.email.trim() } : {}),
  };
}

export function accountValid(v: AccountValues) {
  return v.name.trim() !== "" && v.phone.trim() !== "";
}

/** Name, surname, phone (the mobile sign-in) and optional e-mail of a person. */
export function AccountFields({
  idPrefix,
  value,
  onChange,
  touched,
}: {
  idPrefix: string;
  value: AccountValues;
  onChange: (next: AccountValues) => void;
  touched: boolean;
}) {
  const { t } = useLocale();
  const field = (key: keyof AccountValues) => ({
    id: `${idPrefix}-${key}`,
    value: value[key],
    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      onChange({ ...value, [key]: e.target.value }),
  });
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-name`}>
          {t("organizations.accounts.fields.name")}
        </Label>
        <Input {...field("name")} autoComplete="given-name" />
        {touched && !value.name.trim() ? (
          <p className="text-destructive text-sm">
            {t("organizations.accounts.errors.name_required")}
          </p>
        ) : null}
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-surname`}>
          {t("organizations.accounts.fields.surname")}
        </Label>
        <Input {...field("surname")} autoComplete="family-name" />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-phone`}>
          {t("organizations.accounts.fields.phone")}
        </Label>
        <Input
          {...field("phone")}
          type="tel"
          inputMode="tel"
          placeholder="+90 5xx xxx xx xx"
          autoComplete="tel"
        />
        <p className="text-muted-foreground text-sm">
          {t("organizations.accounts.fields.phone_hint")}
        </p>
        {touched && !value.phone.trim() ? (
          <p className="text-destructive text-sm">
            {t("organizations.accounts.errors.phone_required")}
          </p>
        ) : null}
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-email`}>
          {t("organizations.accounts.fields.email")}
        </Label>
        <Input {...field("email")} type="email" autoComplete="email" />
      </div>
    </div>
  );
}

/** What was opened, with the sign-in hint; "add another" resets the form. */
export function AccountCreated({
  title,
  lines,
  onAnother,
}: {
  title: string;
  lines: string[];
  onAnother: () => void;
}) {
  const { t } = useLocale();
  return (
    <Card data-testid="account-created">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CheckCircle2 className="text-primary size-5" aria-hidden />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {lines.map((l) => (
          <p key={l}>{l}</p>
        ))}
        <p className="text-muted-foreground text-sm">
          {t("organizations.accounts.sign_in_hint")}
        </p>
        <Button type="button" variant="outline" onClick={onAnother}>
          {t("organizations.accounts.add_another")}
        </Button>
      </CardContent>
    </Card>
  );
}
