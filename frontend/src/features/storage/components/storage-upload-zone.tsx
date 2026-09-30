"use client";

import { UploadCloud } from "lucide-react";
import { useCallback, useRef, useState } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function StorageUploadZone({
  disabled,
  onFiles,
  className,
}: {
  disabled?: boolean;
  onFiles: (files: File[]) => void;
  className?: string;
}) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const pick = useCallback(() => {
    if (!disabled) inputRef.current?.click();
  }, [disabled]);

  const handleFiles = useCallback(
    (list: FileList | null) => {
      if (!list?.length || disabled) return;
      onFiles(Array.from(list));
    },
    [disabled, onFiles],
  );

  return (
    <div
      className={cn(
        "relative rounded-lg border border-dashed p-6 text-center transition-colors",
        dragging && "border-primary bg-primary/5",
        disabled && "pointer-events-none opacity-50",
        className,
      )}
      onDragEnter={(event) => {
        event.preventDefault();
        setDragging(true);
      }}
      onDragOver={(event) => event.preventDefault()}
      onDragLeave={() => setDragging(false)}
      onDrop={(event) => {
        event.preventDefault();
        setDragging(false);
        handleFiles(event.dataTransfer.files);
      }}
      onClick={pick}
      role="button"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") pick();
      }}
    >
      <input
        ref={inputRef}
        type="file"
        multiple
        className="hidden"
        onChange={(event) => handleFiles(event.target.files)}
      />
      <UploadCloud className="text-muted-foreground mx-auto mb-2 size-8" />
      <p className="font-medium">{t("storage.drop_title")}</p>
      <p className="text-muted-foreground text-sm">{t("storage.drop_hint")}</p>
    </div>
  );
}
