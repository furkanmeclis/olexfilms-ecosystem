"use client";

import { Copy, Link2, QrCode, Trash2 } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useCreatePublicLink,
  useCreateSignedUrl,
  useRevokeLink,
  useStorageLinks,
} from "@/features/storage/hooks/use-storage-sharing";
import { storageService } from "@/features/storage/services/storage.service";
import type { StorageLink, StorageObject } from "@/features/storage/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type ShareTab = "signed" | "public" | "links";

function LinkResultCard({
  label,
  url,
  expiresAt,
  onCopy,
  onQr,
}: {
  label: string;
  url: string;
  expiresAt?: string;
  onCopy: () => void;
  onQr?: () => void;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-muted/40 space-y-2 rounded-md border p-3 text-sm">
      <p className="font-medium">{label}</p>
      <p className="font-mono text-xs break-all">{url}</p>
      {expiresAt ? (
        <p className="text-muted-foreground text-xs">
          {t("storage.expires_at")}: {expiresAt}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" onClick={onCopy}>
          <Copy /> {t("storage.copy_url")}
        </Button>
        {onQr ? (
          <Button type="button" size="sm" variant="outline" onClick={onQr}>
            <QrCode /> {t("storage.generate_qr")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function SignedShareForm({
  object,
  onQr,
}: {
  object: StorageObject;
  onQr: (url: string) => void;
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

  const sharePageUrl = result
    ? storageService.signedSharePageUrl(result.token ?? result.url)
    : "";

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("storage.signed_description")}
      </p>
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
      <Button
        type="button"
        disabled={create.isPending}
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
      >
        <Link2 /> {t("storage.generate_signed")}
      </Button>
      {result ? (
        <>
          <LinkResultCard
            label={t("storage.share_page_url")}
            url={sharePageUrl}
            expiresAt={result.expires_at}
            onCopy={() => {
              void navigator.clipboard.writeText(sharePageUrl);
              appToast.success(t("storage.copied"));
            }}
            onQr={() => onQr(sharePageUrl)}
          />
          <LinkResultCard
            label={t("storage.direct_file_url")}
            url={result.url}
            expiresAt={result.expires_at}
            onCopy={() => {
              void navigator.clipboard.writeText(result.url);
              appToast.success(t("storage.copied"));
            }}
          />
        </>
      ) : null}
    </div>
  );
}

function PublicShareForm({
  object,
  onQr,
}: {
  object: StorageObject;
  onQr: (url: string) => void;
}) {
  const { t } = useLocale();
  const [slug, setSlug] = useState("");
  const [expiresIn, setExpiresIn] = useState<number | undefined>();
  const [canDownload, setCanDownload] = useState(true);
  const [canView, setCanView] = useState(true);
  const [result, setResult] = useState<StorageLink | null>(null);
  const create = useCreatePublicLink();

  const sharePageUrl = result?.slug
    ? storageService.publicSharePageUrl(result.slug)
    : "";

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("storage.public_description")}
      </p>
      <div className="space-y-1">
        <Label htmlFor="share-slug">{t("storage.custom_slug")}</Label>
        <Input
          id="share-slug"
          value={slug}
          onChange={(event) => setSlug(event.target.value)}
          placeholder={t("storage.slug_optional")}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="share-exp">{t("storage.expiration")}</Label>
        <Input
          id="share-exp"
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
      <Button
        type="button"
        disabled={create.isPending}
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
      >
        <Link2 /> {t("storage.generate_public")}
      </Button>
      {result ? (
        <>
          <LinkResultCard
            label={t("storage.share_page_url")}
            url={sharePageUrl}
            expiresAt={result.expires_at}
            onCopy={() => {
              void navigator.clipboard.writeText(sharePageUrl);
              appToast.success(t("storage.copied"));
            }}
            onQr={() => onQr(sharePageUrl)}
          />
          <LinkResultCard
            label={t("storage.direct_file_url")}
            url={result.url}
            expiresAt={result.expires_at}
            onCopy={() => {
              void navigator.clipboard.writeText(result.url);
              appToast.success(t("storage.copied"));
            }}
          />
        </>
      ) : null}
    </div>
  );
}

function ExistingLinksPanel({
  object,
  canWrite,
  onQr,
}: {
  object: StorageObject;
  canWrite: boolean;
  onQr: (url: string) => void;
}) {
  const { t, locale } = useLocale();
  const linksQuery = useStorageLinks(object.key);
  const revoke = useRevokeLink();

  const copy = (url: string) => {
    void navigator.clipboard.writeText(url).then(() => {
      appToast.success(t("storage.copied"));
    });
  };

  const displayUrl = (link: StorageLink) => {
    if (link.kind === "public" && link.slug) {
      return storageService.publicSharePageUrl(link.slug);
    }
    return link.url;
  };

  if (linksQuery.isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("storage.loading_links")}
      </p>
    );
  }

  const links = linksQuery.data ?? [];
  if (!links.length) {
    return (
      <p className="text-muted-foreground text-sm">{t("storage.no_links")}</p>
    );
  }

  return (
    <div className="space-y-3">
      {links.map((link) => (
        <div
          key={link.uuid}
          className="space-y-2 rounded-md border p-3 text-sm"
        >
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">
              {link.kind === "signed"
                ? t("storage.link_signed")
                : t("storage.link_public")}
            </Badge>
            <Badge
              variant={
                link.status === "active"
                  ? "default"
                  : link.status === "expired"
                    ? "secondary"
                    : "danger"
              }
            >
              {t(
                `storage.link_status_${link.status}` as "storage.link_status_active",
              )}
            </Badge>
          </div>
          {link.url ? (
            <p className="font-mono text-xs break-all">{displayUrl(link)}</p>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("storage.signed_url_hidden")}
            </p>
          )}
          <p className="text-muted-foreground text-xs">
            {t("storage.created_at")}:{" "}
            {datetime(link.created_at, "dd.MM.yyyy HH:mm", locale)}
          </p>
          <div className="flex flex-wrap gap-2">
            {link.url ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => copy(displayUrl(link))}
                >
                  <Copy /> {t("storage.copy_url")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => onQr(displayUrl(link))}
                >
                  <QrCode /> {t("storage.generate_qr")}
                </Button>
              </>
            ) : null}
            {canWrite && link.status === "active" ? (
              <Button
                type="button"
                size="sm"
                variant="destructive"
                onClick={() =>
                  revoke.mutate({ uuid: link.uuid, key: object.key })
                }
              >
                <Trash2 /> {t("storage.revoke_link")}
              </Button>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  );
}

export function StorageShareDialog({
  object,
  open,
  tab,
  onOpenChange,
  onQr,
  canWrite,
}: {
  object: StorageObject | null;
  open: boolean;
  tab: ShareTab;
  onOpenChange: (open: boolean) => void;
  onQr: (url: string) => void;
  canWrite: boolean;
}) {
  const { t } = useLocale();

  if (!object || object.kind === "folder") return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("storage.share_dialog_title")}</DialogTitle>
          <p className="text-muted-foreground truncate text-sm">
            {object.name}
          </p>
        </DialogHeader>
        {open ? (
          <Tabs key={`${object.key}:${tab}`} defaultValue={tab}>
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="signed">
                {t("storage.tab_signed")}
              </TabsTrigger>
              <TabsTrigger value="public">
                {t("storage.tab_public")}
              </TabsTrigger>
              <TabsTrigger value="links">{t("storage.tab_links")}</TabsTrigger>
            </TabsList>
            <TabsContent value="signed" className="mt-4">
              {canWrite ? (
                <SignedShareForm object={object} onQr={onQr} />
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("storage.share_write_required")}
                </p>
              )}
            </TabsContent>
            <TabsContent value="public" className="mt-4">
              {canWrite ? (
                <PublicShareForm object={object} onQr={onQr} />
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("storage.share_write_required")}
                </p>
              )}
            </TabsContent>
            <TabsContent value="links" className="mt-4">
              <ExistingLinksPanel
                object={object}
                canWrite={canWrite}
                onQr={onQr}
              />
            </TabsContent>
          </Tabs>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

export type { ShareTab as StorageShareTab };
