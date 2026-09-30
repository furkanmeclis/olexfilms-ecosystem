"use client";

import { useRef } from "react";
import { Loader2, Upload } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Localized, on-brand replacement for a bare <input type="file"> (whose
 * "Choose File / No file chosen" text follows the browser language).
 */
export function FilePickButton({
  id,
  accept,
  label,
  hint,
  disabled,
  pending,
  onFile,
}: {
  id?: string;
  accept?: string;
  label?: string;
  hint?: string;
  disabled?: boolean;
  pending?: boolean;
  onFile: (file: File) => void;
}) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        ref={inputRef}
        id={id}
        type="file"
        accept={accept}
        className="sr-only"
        tabIndex={-1}
        disabled={disabled || pending}
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) onFile(file);
          event.target.value = "";
        }}
      />
      <Button
        type="button"
        variant="outline"
        disabled={disabled || pending}
        onClick={() => inputRef.current?.click()}
      >
        {pending ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Upload className="size-4" />
        )}
        {label ?? t("common.choose_file")}
      </Button>
      {hint ? (
        <span className="text-muted-foreground text-xs">{hint}</span>
      ) : null}
    </div>
  );
}
