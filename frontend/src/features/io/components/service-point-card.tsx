"use client";

import { Store } from "lucide-react";
import { useState } from "react";

import { FilePickButton } from "@/components/common/file-pick-button";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { apiConfig } from "@/config/api";
import type { AppSettings } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

export type ServicePointCardProps = {
  servicePoint: NonNullable<AppSettings["service_point"]>;
  canWrite: boolean;
  isSaving: boolean;
  isLogoBusy: boolean;
  onSave: (body: { enabled: boolean; name: string }) => void;
  onUploadLogo: (file: File) => void;
  onRemoveLogo: () => void;
};

/**
 * TEC-1003: the brand center's own service point (AFM Florya). While it is
 * on, its name and logo replace the organization's on customer surfaces
 * (warranty page and PDF, portal, WhatsApp); the panel keeps the
 * organization name.
 */
export function ServicePointCard({
  servicePoint,
  canWrite,
  isSaving,
  isLogoBusy,
  onSave,
  onUploadLogo,
  onRemoveLogo,
}: ServicePointCardProps) {
  const { t } = useLocale();
  const [enabled, setEnabled] = useState(servicePoint.enabled);
  const [name, setName] = useState(servicePoint.name ?? "");
  const logoSrc = servicePoint.logo_url
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${servicePoint.logo_url}`
    : null;
  const dirty =
    enabled !== servicePoint.enabled ||
    name.trim() !== (servicePoint.name ?? "");

  return (
    <Card data-testid="service-point-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Store className="size-4" />
          {t("settings.service_point.title")}
        </CardTitle>
        <CardDescription>{t("settings.service_point.hint")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-3">
          <Switch
            id="service-point-enabled"
            checked={enabled}
            disabled={!canWrite}
            onCheckedChange={setEnabled}
          />
          <Label htmlFor="service-point-enabled">
            {t("settings.service_point.enabled")}
          </Label>
        </div>
        <div className="max-w-md space-y-2">
          <Label htmlFor="service-point-name">
            {t("settings.service_point.name")}
          </Label>
          <Input
            id="service-point-name"
            value={name}
            maxLength={200}
            disabled={!canWrite}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="flex flex-wrap items-end gap-3">
          {logoSrc ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={logoSrc}
              alt={name}
              className="h-12 w-auto max-w-[160px] object-contain"
            />
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("settings.service_point.no_logo")}
            </p>
          )}
          <FilePickButton
            id="service-point-logo"
            accept="image/png,image/jpeg,image/webp,image/svg+xml"
            hint={t("common.image_formats")}
            disabled={!canWrite}
            pending={isLogoBusy}
            onFile={onUploadLogo}
          />
          {servicePoint.logo_url && canWrite ? (
            <Button
              type="button"
              variant="outline"
              disabled={isLogoBusy}
              onClick={onRemoveLogo}
            >
              {t("settings.remove_logo")}
            </Button>
          ) : null}
        </div>
        {canWrite ? (
          <Button
            type="button"
            disabled={!dirty || isSaving}
            onClick={() => onSave({ enabled, name: name.trim() })}
          >
            {t("settings.save")}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
