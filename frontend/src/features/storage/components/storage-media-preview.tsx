"use client";

import type { PreviewKind } from "@/features/storage/lib/preview";
import { useLocale } from "@/providers/locale-provider";

export function StorageMediaPreview({
  url,
  name,
  kind,
  downloadUrl,
}: {
  url: string;
  name: string;
  kind: PreviewKind;
  downloadUrl?: string;
}) {
  const { t } = useLocale();

  if (kind === "image") {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={url}
        alt={name}
        className="max-h-full max-w-full object-contain"
      />
    );
  }

  if (kind === "video") {
    return <video src={url} controls className="max-h-full max-w-full" />;
  }

  if (kind === "audio") {
    return <audio src={url} controls className="w-full max-w-lg" />;
  }

  if (kind === "pdf" || kind === "text") {
    return <iframe src={url} title={name} className="h-[60vh] w-full" />;
  }

  return (
    <div className="space-y-3 text-center">
      <p className="font-medium">{t("storage.preview_unavailable")}</p>
      {downloadUrl ? (
        <a
          href={downloadUrl}
          className="text-primary inline-flex text-sm underline-offset-4 hover:underline"
        >
          {t("storage.download")}
        </a>
      ) : null}
    </div>
  );
}
