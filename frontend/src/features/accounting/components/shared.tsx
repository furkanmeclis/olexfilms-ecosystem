"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, type ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Label } from "@/components/ui/label";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type FinanceEntry,
} from "@/features/accounting/services/accounting.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type SelectOption = { value: string; label: string };

/** Native select (keyboard and screen-reader friendly, testable). */
export function NativeSelect({
  id,
  name,
  label,
  value,
  options,
  onChange,
  placeholder,
  error,
  disabled,
  className,
}: {
  id: string;
  name?: string;
  label: string;
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  error?: string;
  disabled?: boolean;
  className?: string;
}) {
  const errorId = error ? `${id}-error` : undefined;
  return (
    <div className={cn("grid gap-1.5", className)}>
      <Label htmlFor={id}>{label}</Label>
      <select
        id={id}
        name={name ?? id}
        value={value}
        disabled={disabled}
        aria-invalid={error ? true : undefined}
        aria-describedby={errorId}
        onChange={(e) => onChange(e.target.value)}
        className="border-input bg-background aria-invalid:border-destructive h-9 w-full rounded-md border px-2 text-sm disabled:opacity-50"
      >
        {placeholder !== undefined ? (
          <option value="">{placeholder}</option>
        ) : null}
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      {error ? (
        <p id={errorId} className="text-destructive text-xs">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} className="text-destructive text-xs">
      {message}
    </p>
  );
}

/** Amount in its ISO 4217 currency, locale formatted (Intl). */
export function Money({
  amount,
  currency,
  className,
}: {
  amount: string | number;
  currency: string;
  className?: string;
}) {
  const { format } = useLocale();
  const value = typeof amount === "number" ? amount : Number(amount);
  return (
    <span
      dir="ltr"
      className={cn("tabular-nums", value < 0 && "text-destructive", className)}
    >
      {format.currency(value, currency)}
    </span>
  );
}

/**
 * Ledger amount; when the entry was written in another currency the
 * original amount and the frozen rate (K7) follow on a second line.
 */
export function EntryAmount({ entry }: { entry: FinanceEntry }) {
  const { t, format } = useLocale();
  const foreign = entry.orig_currency !== entry.currency;
  return (
    <div className="text-end">
      <Money amount={entry.amount} currency={entry.currency} />
      {foreign ? (
        <div className="text-muted-foreground text-xs">
          <Money amount={entry.orig_amount} currency={entry.orig_currency} />{" "}
          <span data-testid="entry-rate">
            {t("accounting.entries.rate", {
              rate: format.number(Number(entry.rate), {
                maximumFractionDigits: 6,
              }),
              date: format.date(entry.rate_date),
            })}
          </span>
        </div>
      ) : null}
    </div>
  );
}

/** Reversal markers: the mirror row and the row it reversed. */
export function EntryStatus({ entry }: { entry: FinanceEntry }) {
  const { t } = useLocale();
  const chips: ReactNode[] = [];
  if (entry.reversal_of_uuid) {
    chips.push(
      <span key="reversal" data-testid="entry-reversal">
        <StatusChip label={t("accounting.entries.reversal")} tone="warning" />
      </span>,
    );
  }
  if (entry.voided) {
    chips.push(
      <span key="voided" data-testid="entry-voided">
        <StatusChip label={t("accounting.entries.voided")} tone="danger" />
      </span>,
    );
  }
  if (!chips.length) return null;
  return <div className="flex flex-wrap gap-1">{chips}</div>;
}

/** Category key → label in the request language (backend catalog). */
export function useCategoryLabels(orgUuid: string, enabled: boolean) {
  const query = useQuery({
    queryKey: accountingKeys.categories(orgUuid),
    queryFn: () => accountingService.listCategories(),
    enabled: enabled && Boolean(orgUuid),
    staleTime: 10 * 60 * 1000,
  });
  return useMemo(() => {
    const map = new Map<string, string>();
    for (const c of query.data?.items ?? []) map.set(c.key, c.label);
    return map;
  }, [query.data]);
}

/** Cari balance: positive = receivable, negative = payable (TEC-172). */
export function BalanceLabel({ balance }: { balance: string }) {
  const { t } = useLocale();
  const value = Number(balance);
  if (value > 0) {
    return (
      <StatusChip label={t("accounting.balance.receivable")} tone="success" />
    );
  }
  if (value < 0) {
    return <StatusChip label={t("accounting.balance.payable")} tone="danger" />;
  }
  return <StatusChip label={t("accounting.balance.settled")} />;
}

/** RFC 4122 v4 uuid for idempotency keys (crypto.randomUUID when present). */
export function newIdempotencyKey(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) return c.randomUUID();
  const bytes = new Uint8Array(16);
  if (c?.getRandomValues) c.getRandomValues(bytes);
  else for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
  bytes[6] = (bytes[6]! & 0x0f) | 0x40;
  bytes[8] = (bytes[8]! & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0"));
  return `${hex.slice(0, 4).join("")}-${hex.slice(4, 6).join("")}-${hex
    .slice(6, 8)
    .join("")}-${hex.slice(8, 10).join("")}-${hex.slice(10).join("")}`;
}
