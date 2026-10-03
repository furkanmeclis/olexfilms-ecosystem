"use client";

import { Loader2, Printer } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Downloads a label sheet PDF (TEC-202): a batch's `labels_url`, a unit's
 * `label_url` or the location QR sheet. `path` is the backend path
 * (`/v1/...`), fetched through the BFF.
 */
export function LabelButton({
  path,
  filename,
  label,
  size = "sm",
  variant = "outline",
  testId,
}: {
  path: string;
  filename: string;
  label?: string;
  size?: "sm" | "default" | "icon-sm";
  variant?: "outline" | "ghost" | "default";
  testId?: string;
}) {
  const { t } = useLocale();
  const [busy, setBusy] = useState(false);
  const text = label ?? t("warehouse.labels.print");

  const onClick = async () => {
    setBusy(true);
    try {
      const { blob, filename: served } = await platformDownloadFile(path);
      triggerBrowserDownload(blob, served ?? filename);
    } catch {
      appToast.error(t("warehouse.labels.failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      type="button"
      size={size}
      variant={variant}
      disabled={busy}
      aria-busy={busy}
      aria-label={size === "icon-sm" ? text : undefined}
      title={text}
      onClick={() => void onClick()}
      data-testid={testId}
    >
      {busy ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <Printer className="size-4" />
      )}
      {size === "icon-sm" ? null : text}
    </Button>
  );
}
