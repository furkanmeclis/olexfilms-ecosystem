"use client";

import { Building2, ImagePlus, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { storageConfig } from "@/config/storage";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const LOGO_ACCEPT = storageConfig.acceptedImageTypes.join(",");

export type LogoControlProps = {
  logoUrl?: string | null;
  onUpload?: (file: File) => Promise<void> | void;
  onClear?: () => Promise<void> | void;
  disabled?: boolean;
  /** Hide upload/clear actions (read-only preview). */
  readOnly?: boolean;
  size?: "md" | "lg";
  className?: string;
};

export function LogoControl({
  logoUrl,
  onUpload,
  onClear,
  disabled = false,
  readOnly = false,
  size = "lg",
  className,
}: LogoControlProps) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState(false);
  const [bust, setBust] = useState(0);

  const src = resolveLogoSrc(logoUrl, bust || undefined);
  const hasLogo = Boolean(logoUrl);
  const busy = pending || disabled;
  const avatarClass = size === "lg" ? "size-20" : "size-16";
  const showActions = !readOnly && Boolean(onUpload);

  const validate = (file: File): boolean => {
    if (
      !(storageConfig.acceptedImageTypes as readonly string[]).includes(
        file.type,
      )
    ) {
      appToast.error(t("branding.logo.invalid_type"));
      return false;
    }
    if (file.size > storageConfig.logoMaxFileSizeBytes) {
      appToast.error(t("branding.logo.too_large"));
      return false;
    }
    return true;
  };

  const handleFile = async (file: File | undefined) => {
    if (!file || busy || !onUpload) return;
    if (!validate(file)) return;
    setPending(true);
    try {
      await onUpload(file);
      setBust(Date.now());
    } finally {
      setPending(false);
    }
  };

  const handleClear = async () => {
    if (!onClear || busy || !hasLogo) return;
    setPending(true);
    try {
      await onClear();
      setBust(Date.now());
    } finally {
      setPending(false);
    }
  };

  return (
    <div className={cn("flex flex-col items-start gap-2", className)}>
      <Avatar className={avatarClass}>
        {src ? <AvatarImage src={src} alt="" /> : null}
        <AvatarFallback>
          <Building2
            className={cn(
              "text-muted-foreground",
              size === "lg" ? "size-8" : "size-6",
            )}
          />
        </AvatarFallback>
      </Avatar>

      {showActions ? (
        <div className="flex flex-wrap items-center gap-2">
          <input
            ref={inputRef}
            type="file"
            accept={LOGO_ACCEPT}
            className="sr-only"
            disabled={busy}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              void handleFile(file);
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => inputRef.current?.click()}
          >
            <ImagePlus className="size-4" />
            {pending
              ? t("branding.logo.uploading")
              : hasLogo
                ? t("branding.logo.replace")
                : t("branding.logo.upload")}
          </Button>
          {hasLogo && onClear ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => void handleClear()}
            >
              <Trash2 className="size-4" />
              {t("branding.logo.clear")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
