"use client";

import { ChevronDown, Download, FileSpreadsheet, Upload } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { IMPORT_ACCEPT } from "@/features/io/lib/import-file";
import type { ImportFormat } from "@/features/io/types";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

const OTHER_SAMPLE_FORMATS: ImportFormat[] = ["csv", "tsv", "json"];

type ImportFileStepProps = {
  file: File | null;
  format: ImportFormat | null;
  onFile: (file: File | null) => void;
  onDownloadSample: (format: ImportFormat) => void;
  /** Resource-specific options (rendered under the drop zone). */
  extraOptions?: ReactNode;
};

/** Step 1: how-to, template download and the file drop zone. */
export function ImportFileStep({
  file,
  format,
  onFile,
  onDownloadSample,
  extraOptions,
}: ImportFileStepProps) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const unsupported = Boolean(file && !format);

  return (
    <div className="space-y-6">
      <div className="bg-muted/40 grid gap-4 rounded-lg border p-4 md:grid-cols-[1fr_auto] md:items-center">
        <div className="space-y-2">
          <p className="text-sm font-medium">{t("imports.howto.title")}</p>
          <ol className="text-muted-foreground list-decimal space-y-1 ps-5 text-sm">
            <li>{t("imports.howto.step1")}</li>
            <li>{t("imports.howto.step2")}</li>
            <li>{t("imports.howto.step3")}</li>
          </ol>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            onClick={() => onDownloadSample("xlsx")}
            data-testid="import-download-template"
          >
            <Download className="size-4" />
            {t("imports.download_template")}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="ghost" size="sm">
                {t("imports.other_formats")}
                <ChevronDown className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {OTHER_SAMPLE_FORMATS.map((f) => (
                <DropdownMenuItem key={f} onSelect={() => onDownloadSample(f)}>
                  {t("imports.download_sample", {
                    format: t(`imports.formats.${f}`),
                  })}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <label
        htmlFor="import-file"
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          onFile(e.dataTransfer.files?.[0] ?? null);
        }}
        className={cn(
          "flex cursor-pointer flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed px-6 py-10 text-center transition-colors",
          dragging ? "border-primary bg-primary/5" : "hover:bg-muted/40",
          unsupported && "border-destructive",
        )}
        data-testid="import-dropzone"
      >
        {file ? (
          <>
            <FileSpreadsheet className="text-primary size-10" />
            <div className="space-y-1">
              <p className="font-medium break-all">{file.name}</p>
              <div className="text-muted-foreground flex flex-wrap items-center justify-center gap-2 text-xs">
                <span>{fileSize(file.size)}</span>
                {format ? (
                  <Badge
                    variant="secondary"
                    data-testid="import-detected-format"
                  >
                    {t(`imports.formats.${format}`)}
                  </Badge>
                ) : null}
              </div>
            </div>
            {unsupported ? (
              <p className="text-destructive text-sm">
                {t("imports.file_unsupported")}
              </p>
            ) : null}
            <span className="text-primary text-sm underline-offset-4 hover:underline">
              {t("imports.file_change")}
            </span>
          </>
        ) : (
          <>
            <Upload className="text-muted-foreground size-10" />
            <div className="space-y-1">
              <p className="font-medium">{t("imports.dropzone_title")}</p>
              <p className="text-muted-foreground text-xs">
                {t("imports.file_hint")}
              </p>
            </div>
          </>
        )}
        <input
          ref={inputRef}
          id="import-file"
          type="file"
          accept={IMPORT_ACCEPT}
          className="sr-only"
          onChange={(e) => {
            onFile(e.target.files?.[0] ?? null);
            // Picking the same file again still fires onChange.
            if (inputRef.current) inputRef.current.value = "";
          }}
        />
      </label>

      {extraOptions ? <div className="space-y-3">{extraOptions}</div> : null}
    </div>
  );
}
