"use client";

import { Download } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { StorageMediaPreview } from "@/features/storage/components/storage-media-preview";
import { StorageSpreadsheetPreviewPanel } from "@/features/storage/components/storage-spreadsheet-preview";
import { usePublicSpreadsheetPreview } from "@/features/storage/hooks/use-storage-spreadsheet-preview";
import {
  fileKindFromMime,
  parseContentDispositionFilename,
  previewKindFromMime,
} from "@/features/storage/lib/preview";
import { useLocale } from "@/providers/locale-provider";

type PublicFileMeta = {
  name: string;
  mime: string;
  size?: number;
};

function PublicFileContent({
  streamUrl,
  downloadUrl,
}: {
  streamUrl: string;
  downloadUrl: string;
}) {
  const { t } = useLocale();
  const [meta, setMeta] = useState<PublicFileMeta | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    void fetch(streamUrl, { method: "HEAD", credentials: "omit" })
      .then((response) => {
        if (cancelled) return;
        if (!response.ok) {
          if (response.status === 403) {
            setError(t("storage.share_forbidden"));
            return;
          }
          if (response.status === 404) {
            setError(t("storage.share_not_found"));
            return;
          }
          setError(t("storage.share_error"));
          return;
        }

        const mime =
          response.headers.get("content-type")?.split(";")[0]?.trim() ??
          "application/octet-stream";
        const name =
          parseContentDispositionFilename(
            response.headers.get("content-disposition"),
          ) ?? t("storage.shared_file");
        const length = response.headers.get("content-length");

        setMeta({
          name,
          mime,
          size: length ? Number(length) : undefined,
        });
      })
      .catch(() => {
        if (!cancelled) setError(t("storage.share_error"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [streamUrl, t]);

  const previewKind = useMemo(
    () => previewKindFromMime(meta?.mime ?? ""),
    [meta?.mime],
  );
  const spreadsheetPreview = usePublicSpreadsheetPreview(
    streamUrl,
    meta?.name ?? "",
    meta?.mime ?? "",
    Boolean(meta) && previewKind === "spreadsheet",
  );

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-muted-foreground text-sm">
          {t("storage.share_loading")}
        </p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <ErrorState title={error} />
      </div>
    );
  }

  if (!meta) {
    return null;
  }

  return (
    <div className="bg-background min-h-screen">
      <header className="border-b px-4 py-4 sm:px-6">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-muted-foreground text-sm">
              {t("storage.shared_file")}
            </p>
            <h1 className="truncate text-lg font-semibold">{meta.name}</h1>
            <p className="text-muted-foreground text-xs">
              {t(
                `storage.kind_${fileKindFromMime(meta.mime)}` as "storage.kind_unknown",
              )}{" "}
              · {meta.mime}
            </p>
          </div>
          <Button type="button" asChild>
            <a href={downloadUrl}>
              <Download /> {t("storage.download")}
            </a>
          </Button>
        </div>
      </header>
      <main className="mx-auto flex max-w-5xl justify-center p-4 sm:p-6">
        <div className="bg-muted/20 flex min-h-[60vh] w-full items-center justify-center overflow-auto rounded-xl border p-4">
          {previewKind === "spreadsheet" ? (
            <StorageSpreadsheetPreviewPanel
              sheet={spreadsheetPreview.data}
              isLoading={spreadsheetPreview.isLoading}
              isError={spreadsheetPreview.isError}
              onRetry={() => void spreadsheetPreview.refetch()}
            />
          ) : (
            <StorageMediaPreview
              url={streamUrl}
              name={meta.name}
              kind={previewKind}
              downloadUrl={downloadUrl}
            />
          )}
        </div>
      </main>
    </div>
  );
}

export function StoragePublicViewer({
  streamUrl,
  downloadUrl,
}: {
  streamUrl: string;
  downloadUrl: string;
}) {
  return (
    <PublicFileContent
      key={streamUrl}
      streamUrl={streamUrl}
      downloadUrl={downloadUrl}
    />
  );
}
