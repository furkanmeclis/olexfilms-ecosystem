"use client";

import { ImagePlus, X } from "lucide-react";
import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  CLAIM_PHOTO_TYPES,
  checkClaimPhoto,
} from "@/features/warranty-claims/lib/claims";
import { useLocale } from "@/providers/locale-provider";

/**
 * Multiple photo choice for a claim (JPEG / PNG / WebP, ≤ 12 MB each).
 * Rejected files are named under the input; accepted ones are listed with
 * a remove button. The upload itself is the caller's.
 */
export function ClaimPhotoPicker({
  files,
  onChange,
  disabled = false,
}: {
  files: File[];
  onChange: (files: File[]) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const id = useId();
  const [rejected, setRejected] = useState<string[]>([]);

  const add = (list: FileList | null) => {
    if (!list) return;
    const ok: File[] = [];
    const bad: string[] = [];
    for (const file of Array.from(list)) {
      const check = checkClaimPhoto(file);
      if (check === "ok") ok.push(file);
      else bad.push(t(`warranty.claims.photos.${check}`, { name: file.name }));
    }
    setRejected(bad);
    if (ok.length > 0) onChange([...files, ...ok]);
  };

  return (
    <div className="space-y-2">
      <label
        htmlFor={id}
        className="border-input hover:bg-muted/50 flex cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed p-4 text-sm"
      >
        <ImagePlus className="size-4" />
        {t("warranty.claims.photos.choose")}
      </label>
      <input
        id={id}
        type="file"
        multiple
        accept={CLAIM_PHOTO_TYPES.join(",")}
        className="sr-only"
        data-testid="claim-photo-input"
        disabled={disabled}
        onChange={(e) => {
          add(e.target.files);
          e.target.value = "";
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("warranty.claims.photos.hint")}
      </p>
      {rejected.map((msg) => (
        <p key={msg} className="text-destructive text-xs" role="alert">
          {msg}
        </p>
      ))}
      {files.length > 0 ? (
        <ul className="space-y-1" data-testid="claim-photo-list">
          {files.map((file, i) => (
            <li
              key={`${file.name}-${i}`}
              className="flex items-center justify-between gap-2 text-sm"
            >
              <span className="truncate">{file.name}</span>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-7"
                disabled={disabled}
                aria-label={t("warranty.claims.photos.remove")}
                onClick={() => onChange(files.filter((_, j) => j !== i))}
              >
                <X className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
