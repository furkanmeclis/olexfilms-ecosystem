"use client";

import { Upload, X } from "lucide-react";
import { useCallback, useState } from "react";

import { Button } from "@/components/ui/button";
import { storageConfig } from "@/config/storage";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type UploadFile = {
  id: string;
  file: File;
  progress: number;
  status: "idle" | "uploading" | "done" | "error";
  previewUrl?: string;
  error?: string;
};

type FileUploaderProps = {
  multiple?: boolean;
  accept?: string;
  maxSizeBytes?: number;
  onUpload?: (files: File[]) => Promise<void> | void;
  className?: string;
};

export function FileUploader({
  multiple = true,
  accept,
  maxSizeBytes = storageConfig.maxFileSizeBytes,
  onUpload,
  className,
}: FileUploaderProps) {
  const { t } = useLocale();
  const [items, setItems] = useState<UploadFile[]>([]);
  const [dragging, setDragging] = useState(false);

  const ingest = useCallback(
    async (fileList: FileList | null) => {
      if (!fileList?.length) return;
      const files = Array.from(fileList).filter((f) => f.size <= maxSizeBytes);
      const next: UploadFile[] = files.map((file) => ({
        id: crypto.randomUUID(),
        file,
        progress: 0,
        status: "idle",
        previewUrl: file.type.startsWith("image/")
          ? URL.createObjectURL(file)
          : undefined,
      }));
      setItems((prev) => (multiple ? [...prev, ...next] : next));

      setItems((prev) =>
        prev.map((item) =>
          next.some((n) => n.id === item.id)
            ? { ...item, status: "uploading", progress: 30 }
            : item,
        ),
      );

      try {
        await onUpload?.(files);
        setItems((prev) =>
          prev.map((item) =>
            next.some((n) => n.id === item.id)
              ? { ...item, status: "done", progress: 100 }
              : item,
          ),
        );
      } catch {
        setItems((prev) =>
          prev.map((item) =>
            next.some((n) => n.id === item.id)
              ? { ...item, status: "error", error: t("common.error_generic") }
              : item,
          ),
        );
      }
    },
    [maxSizeBytes, multiple, onUpload, t],
  );

  const retry = async (id: string) => {
    const target = items.find((i) => i.id === id);
    if (!target) return;
    setItems((prev) =>
      prev.map((i) =>
        i.id === id
          ? { ...i, status: "uploading", progress: 30, error: undefined }
          : i,
      ),
    );
    try {
      await onUpload?.([target.file]);
      setItems((prev) =>
        prev.map((i) =>
          i.id === id ? { ...i, status: "done", progress: 100 } : i,
        ),
      );
    } catch {
      setItems((prev) =>
        prev.map((i) =>
          i.id === id
            ? { ...i, status: "error", error: t("common.error_generic") }
            : i,
        ),
      );
    }
  };

  return (
    <div className={cn("space-y-3", className)}>
      <label
        className={cn(
          "border-border flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center transition-colors",
          dragging && "border-primary bg-muted/40",
        )}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          void ingest(e.dataTransfer.files);
        }}
      >
        <Upload className="text-muted-foreground h-5 w-5" />
        <span className="text-muted-foreground text-sm">
          Drag & drop or browse
        </span>
        <input
          type="file"
          className="hidden"
          multiple={multiple}
          accept={accept}
          onChange={(e) => void ingest(e.target.files)}
        />
      </label>

      <ul className="space-y-2">
        {items.map((item) => (
          <li
            key={item.id}
            className="border-border flex items-center gap-3 rounded-md border p-2"
          >
            {item.previewUrl ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={item.previewUrl}
                alt=""
                className="h-10 w-10 rounded object-cover"
              />
            ) : (
              <div className="bg-muted flex h-10 w-10 items-center justify-center rounded text-xs">
                FILE
              </div>
            )}
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm">{item.file.name}</p>
              <p className="text-muted-foreground text-xs">
                {fileSize(item.file.size)} · {item.status}
              </p>
              <div className="bg-muted mt-1 h-1.5 overflow-hidden rounded">
                <div
                  className="bg-primary h-full transition-all"
                  style={{ width: `${item.progress}%` }}
                />
              </div>
            </div>
            {item.status === "error" ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void retry(item.id)}
              >
                {t("common.retry")}
              </Button>
            ) : null}
            <Button
              type="button"
              size="icon"
              variant="ghost"
              onClick={() =>
                setItems((prev) => prev.filter((i) => i.id !== item.id))
              }
            >
              <X className="h-4 w-4" />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
