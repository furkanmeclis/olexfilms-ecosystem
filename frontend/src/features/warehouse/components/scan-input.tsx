"use client";

import { Loader2, ScanLine } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { normalizeScan } from "@/features/warehouse/lib/scan";
import {
  warehouseService,
  type WarehouseScanResult,
} from "@/features/warehouse/services/warehouse.service";
import { scanStatusMessage } from "@/features/warehouse/lib/scan-status";
import { useLocale } from "@/providers/locale-provider";

type ScanInputProps = {
  /** Called with the normalized code; the field clears and refocuses. */
  onScan: (code: string) => unknown;
  expect?: "any" | "location" | "bin" | "unit" | "product";
  warehouseUuid?: string;
  label?: string;
  placeholder?: string;
  hint?: string;
  busy?: boolean;
  disabled?: boolean;
  autoFocus?: boolean;
  /** Prefix of the test ids: `<id>-input`, `<id>-submit`. */
  id?: string;
};

/**
 * Universal scan field (TEC-203). A hardware scanner types the code and
 * sends Enter; a person may type or paste and press the button. The value
 * is normalized (control characters dropped, QR prefix upper-cased), an
 * empty scan is ignored, and the field clears and keeps the focus so the
 * next scan follows without a click.
 */
export function ScanInput({
  onScan,
  expect,
  warehouseUuid,
  label,
  placeholder,
  hint,
  busy = false,
  disabled = false,
  autoFocus = true,
  id = "scan",
}: ScanInputProps) {
  const { t } = useLocale();
  const [value, setValue] = useState("");
  const [empty, setEmpty] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (autoFocus && !disabled) ref.current?.focus();
  }, [autoFocus, disabled]);

  const submit = async (e?: FormEvent) => {
    e?.preventDefault();
    if (busy || disabled) return;
    const code = normalizeScan(value);
    if (!code) {
      setEmpty(true);
      ref.current?.focus();
      return;
    }
    setEmpty(false);
    setScanError(null);
    setValue("");
    if (expect || warehouseUuid) {
      try {
        const result: WarehouseScanResult = await warehouseService.scan(code, {
          expect: expect ?? "any",
          ...(warehouseUuid ? { warehouse_uuid: warehouseUuid } : {}),
        });
        if (result.status !== "ok") {
          setScanError(scanStatusMessage(result, expect ?? "any", t));
          ref.current?.focus();
          return;
        }
      } catch {
        setScanError(t("warehouse.scan.failed"));
        ref.current?.focus();
        return;
      }
    }
    void onScan(code);
    ref.current?.focus();
  };

  const inputId = `${id}-input`;
  return (
    <form onSubmit={submit} className="space-y-1.5" role="search">
      <Label htmlFor={inputId}>{label ?? t("warehouse.scan.label")}</Label>
      <div className="flex gap-2">
        <Input
          ref={ref}
          id={inputId}
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            if (empty) setEmpty(false);
            if (scanError) setScanError(null);
          }}
          placeholder={placeholder ?? t("warehouse.scan.placeholder")}
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          inputMode="text"
          enterKeyHint="go"
          dir="ltr"
          disabled={disabled}
          aria-invalid={empty || Boolean(scanError) || undefined}
          aria-describedby={`${id}-hint`}
          data-testid={inputId}
          className="font-mono"
        />
        <Button
          type="submit"
          disabled={busy || disabled}
          aria-busy={busy}
          data-testid={`${id}-submit`}
        >
          {busy ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <ScanLine className="size-4" />
          )}
          {t("warehouse.scan.submit")}
        </Button>
      </div>
      <p
        id={`${id}-hint`}
        className={
          empty || scanError
            ? "text-destructive text-xs"
            : "text-muted-foreground text-xs"
        }
        role={empty || scanError ? "alert" : undefined}
      >
        {empty
          ? t("warehouse.scan.empty")
          : (scanError ?? hint ?? t("warehouse.scan.hint"))}
      </p>
    </form>
  );
}
