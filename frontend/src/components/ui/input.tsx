import * as React from "react";

import { toLatinDigits } from "@/lib/i18n/digits";
import { cn } from "@/lib/utils/index";

/**
 * TEC-1009: Arabic-Indic / Persian digits typed into a field are stored as
 * Latin digits. The value is rewritten before the field's own handler, so
 * controlled inputs and react-hook-form see the Latin value.
 */
function latinDigitsOnChange(
  onChange: React.ChangeEventHandler<HTMLInputElement> | undefined,
): React.ChangeEventHandler<HTMLInputElement> | undefined {
  if (!onChange) return undefined;
  return (event) => {
    const el = event.target;
    const latin = toLatinDigits(el.value);
    if (latin !== el.value && el.type !== "file") {
      const caret = el.selectionStart;
      el.value = latin;
      if (caret !== null) {
        try {
          el.setSelectionRange(caret, caret);
        } catch {
          // number / email inputs have no selection API
        }
      }
    }
    onChange(event);
  };
}

export const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, type, onChange, ...props }, ref) => (
  <input
    type={type}
    onChange={latinDigitsOnChange(onChange)}
    className={cn(
      "border-border bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-sm transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50",
      className,
    )}
    ref={ref}
    {...props}
  />
));
Input.displayName = "Input";
