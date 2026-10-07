"use client";

import { Infinity as InfinityIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  isUnlimitedQuota,
  parseQuotaInput,
} from "@/features/ai-admin/lib/quota";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type QuotaInputProps = {
  id: string;
  value: number | null;
  onChange: (value: number | null) => void;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
  "aria-describedby"?: string;
};

/**
 * Monthly token quota field: whole non-negative numbers only (a minus sign
 * or decimals are dropped while typing); 0 shows the "unlimited" label.
 */
export function QuotaInput({
  id,
  value,
  onChange,
  disabled,
  invalid,
  className,
  ...aria
}: QuotaInputProps) {
  const { t, format } = useLocale();
  const unlimited = isUnlimitedQuota(value);

  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-center gap-2">
        <Input
          id={id}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          dir="ltr"
          className="tabular-nums"
          value={value == null ? "" : String(value)}
          disabled={disabled}
          aria-invalid={invalid || undefined}
          aria-describedby={aria["aria-describedby"]}
          onKeyDown={(event) => {
            if (event.key === "-" || event.key === "+" || event.key === "e") {
              event.preventDefault();
            }
          }}
          onChange={(event) => onChange(parseQuotaInput(event.target.value))}
        />
        {unlimited ? (
          <Badge
            variant="success"
            className="shrink-0 gap-1"
            data-testid="quota-unlimited"
          >
            <InfinityIcon className="size-3.5" aria-hidden />
            {t("ai_admin.quota.unlimited")}
          </Badge>
        ) : null}
      </div>
      <p className="text-muted-foreground text-xs">
        {value != null && value > 0
          ? t("ai_admin.quota.tokens_per_month", {
              value: format.number(value),
            })
          : t("ai_admin.quota.zero_hint")}
      </p>
    </div>
  );
}
