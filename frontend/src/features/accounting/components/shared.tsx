"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, type ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Label } from "@/components/ui/label";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type FinanceEntry,
} from "@/features/accounting/services/accounting.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type SelectOption = { value: string; label: string };

/** Select item value standing for "" (Radix Select items cannot be ""). */
export const EMPTY_SELECT_VALUE = "__none__";
const EMPTY = EMPTY_SELECT_VALUE;
/** Longer lists get a searchable combobox instead of a plain listbox. */
const COMBOBOX_THRESHOLD = 15;

/**
 * Labelled select: a shadcn Select, or a searchable combobox for long lists.
 * `placeholder` stays a pickable "empty" choice that reports "".
 */
export function FormSelect({
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
      {options.length > COMBOBOX_THRESHOLD ? (
        <AsyncCombobox
          id={id}
          value={value}
          disabled={disabled}
          aria-invalid={error ? true : undefined}
          aria-describedby={errorId}
          placeholder={placeholder}
          clearable={placeholder !== undefined}
          options={options}
          onValueChange={(next) => {
            // Without a placeholder there is no empty choice to go back to.
            if (next || placeholder !== undefined) onChange(next);
          }}
        />
      ) : (
        <Select
          name={name ?? id}
          value={value === "" && placeholder !== undefined ? EMPTY : value}
          disabled={disabled}
          onValueChange={(next) => onChange(next === EMPTY ? "" : next)}
        >
          <SelectTrigger
            id={id}
            data-value={value}
            aria-invalid={error ? true : undefined}
            aria-describedby={errorId}
            className="aria-invalid:border-destructive"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {placeholder !== undefined ? (
              <SelectItem value={EMPTY}>{placeholder}</SelectItem>
            ) : null}
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
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
