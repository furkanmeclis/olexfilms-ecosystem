"use client";

import { Download } from "lucide-react";
import { useState } from "react";
import Link from "next/link";

import {
  EXPORT_PATHS,
  type ExportFormat,
  type IoResource,
} from "@/features/io/types";
import { exportsService } from "@/features/io/services/exports.service";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ExportMenuProps = {
  /** Registered resource; or pass `exportPath` for another endpoint. */
  resource?: IoResource;
  query?: Record<string, string | undefined>;
  disabled?: boolean;
  jobsHref?: string;
  exportPath?: string;
  /** Formats the endpoint accepts (default: pdf, xlsx, csv, json). */
  formats?: ExportFormat[];
};

const FORMATS: ExportFormat[] = ["pdf", "xlsx", "csv", "json"];

export function ExportMenu({
  resource,
  query,
  disabled,
  jobsHref,
  exportPath,
  formats = FORMATS,
}: ExportMenuProps) {
  const { t, locale } = useLocale();
  const [pending, setPending] = useState<ExportFormat | null>(null);

  const handleExport = async (format: ExportFormat) => {
    setPending(format);
    try {
      const cleaned: Record<string, string> = {};
      if (query) {
        for (const [key, value] of Object.entries(query)) {
          if (value) cleaned[key] = value;
        }
      }
      const path = exportPath ?? (resource ? EXPORT_PATHS[resource] : null);
      if (!path) throw new Error("export path missing");
      await exportsService.request(path, {
        format,
        query: cleaned,
        locale,
      });
      appToast.success(t("exports.toast.queued"));
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setPending(null);
    }
  };

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-8"
              disabled={disabled || Boolean(pending)}
              aria-label={t("exports.menu_label")}
            >
              <Download className="size-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{t("exports.menu_label")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        {formats.map((format) => (
          <DropdownMenuItem
            key={format}
            disabled={pending === format}
            onClick={() => void handleExport(format)}
          >
            {t(`exports.formats.${format}`)}
          </DropdownMenuItem>
        ))}
        <DropdownMenuItem asChild>
          <Link href={jobsHref ?? routes.platform.exports.root}>
            {t("exports.view_jobs")}
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
