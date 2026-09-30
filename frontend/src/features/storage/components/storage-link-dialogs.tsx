"use client";

import QRCode from "qrcode";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  useCreatePublicLink,
  useCreateSignedUrl,
} from "@/features/storage/hooks/use-storage-sharing";
import type { StorageLink, StorageObject } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function LinkResult({
  link,
  onCopy,
}: {
  link: StorageLink;
  onCopy: () => void;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-muted/40 space-y-2 rounded-md border p-3 text-sm">
      <p className="font-mono text-xs break-all">{link.url}</p>
      {link.expires_at ? (
        <p className="text-muted-foreground text-xs">
          {t("storage.expires_at")}: {link.expires_at}
        </p>
      ) : null}
      <Button type="button" size="sm" variant="outline" onClick={onCopy}>
        {t("storage.copy_url")}
      </Button>
    </div>
  );
}

function StoragePublicLinkForm({
  object,
}: {
  object: StorageObject;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [slug, setSlug] = useState("");
  const [expiresIn, setExpiresIn] = useState<number | undefined>();
  const [canDownload, setCanDownload] = useState(true);
  const [canView, setCanView] = useState(true);
  const [result, setResult] = useState<StorageLink | null>(null);
  const create = useCreatePublicLink();

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("storage.public_link_title")}</DialogTitle>
      </DialogHeader>
      <div className="space-y-3">
        <div className="space-y-1">
          <Label htmlFor="slug">{t("storage.custom_slug")}</Label>
          <Input
            id="slug"
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="exp">{t("storage.expiration")}</Label>
          <Input
            id="exp"
            type="number"
            placeholder={t("storage.exp_custom")}
            value={expiresIn ?? ""}
            onChange={(event) =>
              setExpiresIn(
                event.target.value ? Number(event.target.value) : undefined,
              )
            }
          />
        </div>
        <div className="flex items-center justify-between">
          <Label>{t("storage.perm_preview")}</Label>
          <Switch checked={canView} onCheckedChange={setCanView} />
        </div>
        <div className="flex items-center justify-between">
          <Label>{t("storage.perm_download")}</Label>
          <Switch checked={canDownload} onCheckedChange={setCanDownload} />
        </div>
        {result ? (
          <LinkResult
            link={result}
            onCopy={() => {
              void navigator.clipboard.writeText(result.url);
              appToast.success(t("storage.copied"));
            }}
          />
        ) : null}
      </div>
      <DialogFooter>
        <Button
          type="button"
          onClick={() =>
            create.mutate(
              {
                key: object.key,
                kind: "public",
                slug: slug || undefined,
                expires_in: expiresIn,
                can_view: canView,
                can_download: canDownload,
              },
              { onSuccess: setResult },
            )
          }
          disabled={create.isPending}
        >
          {t("storage.generate")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function StoragePublicLinkDialog({
  object,
  open,
  onOpenChange,
}: {
  object: StorageObject | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!object) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open ? (
          <StoragePublicLinkForm
            key={object.key}
            object={object}
            onOpenChange={onOpenChange}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function StorageSignedUrlForm({
  object,
}: {
  object: StorageObject;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [expiresIn, setExpiresIn] = useState(3600);
  const [canDownload, setCanDownload] = useState(true);
  const [canView, setCanView] = useState(true);
  const [result, setResult] = useState<StorageLink | null>(null);
  const create = useCreateSignedUrl();

  const presets = [
    { label: t("storage.exp_15m"), value: 900 },
    { label: t("storage.exp_1h"), value: 3600 },
    { label: t("storage.exp_6h"), value: 21600 },
    { label: t("storage.exp_24h"), value: 86400 },
    { label: t("storage.exp_7d"), value: 604800 },
  ];

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("storage.signed_title")}</DialogTitle>
      </DialogHeader>
      <div className="space-y-3">
        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => (
            <Button
              key={preset.value}
              type="button"
              size="sm"
              variant={expiresIn === preset.value ? "default" : "outline"}
              onClick={() => setExpiresIn(preset.value)}
            >
              {preset.label}
            </Button>
          ))}
        </div>
        <div className="flex items-center justify-between">
          <Label>{t("storage.perm_view")}</Label>
          <Switch checked={canView} onCheckedChange={setCanView} />
        </div>
        <div className="flex items-center justify-between">
          <Label>{t("storage.perm_download")}</Label>
          <Switch checked={canDownload} onCheckedChange={setCanDownload} />
        </div>
        {result ? (
          <LinkResult
            link={result}
            onCopy={() => {
              void navigator.clipboard.writeText(result.url);
              appToast.success(t("storage.copied"));
            }}
          />
        ) : null}
      </div>
      <DialogFooter>
        <Button
          type="button"
          onClick={() =>
            create.mutate(
              {
                key: object.key,
                kind: "signed",
                expires_in: expiresIn,
                can_view: canView,
                can_download: canDownload,
              },
              { onSuccess: setResult },
            )
          }
          disabled={create.isPending}
        >
          {t("storage.generate")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function StorageSignedUrlDialog({
  object,
  open,
  onOpenChange,
}: {
  object: StorageObject | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!object) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open ? (
          <StorageSignedUrlForm
            key={object.key}
            object={object}
            onOpenChange={onOpenChange}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function StorageQrContent({
  url,
  downloadName,
}: {
  url: string;
  downloadName: string;
}) {
  const { t } = useLocale();
  const [dataUrl, setDataUrl] = useState("");

  useEffect(() => {
    let cancelled = false;
    void QRCode.toDataURL(url, { margin: 1, width: 240 }).then((next) => {
      if (!cancelled) setDataUrl(next);
    });
    return () => {
      cancelled = true;
    };
  }, [url]);

  return (
    <>
      <div className="flex flex-col items-center gap-3 text-center">
        {dataUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={dataUrl} alt="QR" className="rounded-md border" />
        ) : null}
        <p className="text-muted-foreground text-sm">{t("storage.qr_scan")}</p>
        <p className="font-mono text-xs break-all">{url}</p>
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            void navigator.clipboard.writeText(url);
            appToast.success(t("storage.copied"));
          }}
        >
          {t("storage.copy_url")}
        </Button>
        {dataUrl ? (
          <Button type="button" asChild>
            <a href={dataUrl} download={`${downloadName}-qr.png`}>
              {t("storage.download_qr")}
            </a>
          </Button>
        ) : null}
      </DialogFooter>
    </>
  );
}

export function StorageQrDialog({
  object,
  url,
  open,
  onOpenChange,
}: {
  object: StorageObject | null;
  url: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();

  if (!object) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("storage.qr_title")}</DialogTitle>
        </DialogHeader>
        {open && url ? (
          <StorageQrContent key={url} url={url} downloadName={object.name} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
