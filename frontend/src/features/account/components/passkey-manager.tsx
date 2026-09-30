"use client";

import { signIn as signInPasskey } from "next-auth/webauthn";
import {
  Calendar,
  Clock,
  Cloud,
  Fingerprint,
  KeyRound,
  Loader2,
  Monitor,
  Plus,
  Shield,
  Smartphone,
  Trash2,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { authService, type PasskeySummary } from "@/services/auth.service";

function PasskeyDeviceGlyph({
  deviceType,
  className,
}: {
  deviceType: string;
  className?: string;
}) {
  const normalized = deviceType.toLowerCase();
  if (normalized.includes("multi")) {
    return <Monitor className={className} aria-hidden />;
  }
  if (normalized.includes("single")) {
    return <Smartphone className={className} aria-hidden />;
  }
  return <KeyRound className={className} aria-hidden />;
}

function passkeyDisplayName(item: PasskeySummary, fallback: string) {
  const name = item.name?.trim();
  return name && name.length > 0 ? name : fallback;
}

type PasskeyRowProps = {
  item: PasskeySummary;
  deleting: boolean;
  onRename: (uuid: string, name: string) => Promise<void>;
  onDelete: (uuid: string) => Promise<void>;
};

function PasskeyRow({ item, deleting, onRename, onDelete }: PasskeyRowProps) {
  const { t, locale } = useLocale();
  const [name, setName] = useState(item.name ?? "");
  const [saving, setSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const fallbackName = t("auth.passkey.unnamed");

  const saveName = async () => {
    const next = name.trim();
    const current = item.name?.trim() ?? "";
    if (next === current) return;

    setSaving(true);
    try {
      await onRename(item.uuid, next);
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    await onDelete(item.uuid);
    setDeleteOpen(false);
  };

  return (
    <>
      <li className="border-border bg-card flex flex-col gap-4 rounded-xl border p-4 shadow-xs sm:flex-row sm:items-start">
        <div
          className={cn(
            "flex size-11 shrink-0 items-center justify-center rounded-lg",
            item.backed_up
              ? "bg-emerald-600/10 text-emerald-700 dark:text-emerald-300"
              : "bg-primary/10 text-primary",
          )}
        >
          <PasskeyDeviceGlyph
            deviceType={item.device_type}
            className="size-5"
          />
        </div>

        <div className="min-w-0 flex-1 space-y-3">
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {passkeyDisplayName(item, fallbackName)}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <Badge
                variant={item.backed_up ? "success" : "secondary"}
                className="gap-1"
              >
                {item.backed_up ? (
                  <Cloud className="size-3" aria-hidden />
                ) : (
                  <Smartphone className="size-3" aria-hidden />
                )}
                {item.backed_up
                  ? t("auth.passkey.backed_up")
                  : t("auth.passkey.device_only")}
              </Badge>
              <Badge variant="outline">
                {item.device_type.toLowerCase().includes("multi")
                  ? t("auth.passkey.device_type_multi")
                  : t("auth.passkey.device_type_single")}
              </Badge>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor={`passkey-${item.uuid}`} className="text-xs">
              {t("auth.passkey.name")}
            </Label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                id={`passkey-${item.uuid}`}
                value={name}
                placeholder={t("auth.passkey.name_placeholder")}
                disabled={saving || deleting}
                onChange={(event) => setName(event.target.value)}
                onBlur={() => void saveName()}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    void saveName();
                  }
                }}
              />
              {saving ? (
                <div className="text-muted-foreground flex h-9 items-center gap-2 px-1 text-xs">
                  <Loader2 className="size-3.5 animate-spin" aria-hidden />
                  {t("auth.passkey.saving")}
                </div>
              ) : null}
            </div>
          </div>

          <dl className="text-muted-foreground grid gap-2 text-xs sm:grid-cols-2">
            <div className="flex items-center gap-2">
              <Calendar className="size-3.5 shrink-0" aria-hidden />
              <div>
                <dt className="sr-only">{t("auth.passkey.created_at")}</dt>
                <dd>
                  {t("auth.passkey.created_at")}:{" "}
                  {datetime(item.created_at, "dd MMM yyyy HH:mm", locale)}
                </dd>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Clock className="size-3.5 shrink-0" aria-hidden />
              <div>
                <dt className="sr-only">{t("auth.passkey.last_used_at")}</dt>
                <dd>
                  {t("auth.passkey.last_used_at")}:{" "}
                  {item.last_used_at
                    ? datetime(item.last_used_at, "dd MMM yyyy HH:mm", locale)
                    : t("auth.passkey.never_used")}
                </dd>
              </div>
            </div>
          </dl>
        </div>

        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="text-destructive hover:bg-destructive/10 hover:text-destructive shrink-0 self-start"
          disabled={deleting}
          aria-label={t("auth.passkey.delete")}
          onClick={() => setDeleteOpen(true)}
        >
          {deleting ? (
            <Loader2 className="size-4 animate-spin" aria-hidden />
          ) : (
            <Trash2 className="size-4" aria-hidden />
          )}
        </Button>
      </li>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("auth.passkey.delete_title")}</DialogTitle>
            <DialogDescription>
              {t("auth.passkey.delete_description", {
                name: passkeyDisplayName(item, fallbackName),
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDeleteOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleting}
              onClick={() => void confirmDelete()}
            >
              {deleting ? t("auth.passkey.deleting") : t("auth.passkey.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function PasskeySkeleton() {
  return (
    <li className="border-border flex gap-4 rounded-xl border p-4">
      <Skeleton className="size-11 shrink-0 rounded-lg" />
      <div className="min-w-0 flex-1 space-y-3">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-6 w-28" />
        <Skeleton className="h-9 w-full" />
        <div className="grid gap-2 sm:grid-cols-2">
          <Skeleton className="h-3 w-full" />
          <Skeleton className="h-3 w-full" />
        </div>
      </div>
    </li>
  );
}

export function PasskeyManager() {
  const { t } = useLocale();
  const [registerPending, setRegisterPending] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const passkeysQuery = useQuery({
    queryKey: ["account", "passkeys"],
    queryFn: async () => {
      try {
        const list = await authService.listPasskeys();
        return list.items ?? [];
      } catch (error) {
        toast.error(
          isApiError(error)
            ? error.message || t("common.error_generic")
            : t("common.error_generic"),
        );
        throw error;
      }
    },
  });
  const items = passkeysQuery.data ?? [];
  const loading = passkeysQuery.isPending;

  const registerPasskey = async () => {
    setRegisterPending(true);
    try {
      const result = await signInPasskey("passkey", {
        action: "register",
        redirect: false,
      });
      if (result?.error) {
        toast.error(t("auth.passkey.register_error"));
        return;
      }
      toast.success(t("auth.passkey.register_success"));
      await passkeysQuery.refetch();
    } catch {
      toast.error(t("auth.passkey.register_error"));
    } finally {
      setRegisterPending(false);
    }
  };

  const renamePasskey = async (uuid: string, name: string) => {
    try {
      await authService.renamePasskey(uuid, name.trim() || null);
      toast.success(t("auth.passkey.rename_success"));
      await passkeysQuery.refetch();
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    }
  };

  const deletePasskey = async (uuid: string) => {
    setDeletingId(uuid);
    try {
      await authService.deletePasskey(uuid);
      toast.success(t("auth.passkey.delete_success"));
      await passkeysQuery.refetch();
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="space-y-4">
      <Alert>
        <Shield className="size-4" aria-hidden />
        <AlertDescription>{t("auth.passkey.manage_hint")}</AlertDescription>
      </Alert>

      <div className="flex flex-wrap items-center justify-between gap-3">
        {!loading ? (
          <p className="text-muted-foreground text-sm">
            {items.length > 0
              ? t("auth.passkey.registered_count", { count: items.length })
              : t("auth.passkey.empty")}
          </p>
        ) : (
          <Skeleton className="h-4 w-36" />
        )}

        <Button
          type="button"
          disabled={registerPending || loading}
          onClick={registerPasskey}
        >
          {registerPending ? (
            <Loader2 className="size-4 animate-spin" aria-hidden />
          ) : (
            <Plus aria-hidden />
          )}
          {registerPending
            ? t("auth.passkey.registering")
            : t("auth.passkey.register")}
        </Button>
      </div>

      {loading ? (
        <ul className="space-y-3" aria-busy="true" aria-live="polite">
          <PasskeySkeleton />
          <PasskeySkeleton />
        </ul>
      ) : items.length === 0 ? (
        <div className="border-border flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-10 text-center">
          <div className="bg-muted flex size-12 items-center justify-center rounded-full">
            <Fingerprint className="text-muted-foreground size-6" aria-hidden />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {t("auth.passkey.empty_title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("auth.passkey.empty_body")}
            </p>
          </div>
        </div>
      ) : (
        <ul className="space-y-3">
          {items.map((item) => (
            <PasskeyRow
              key={item.uuid}
              item={item}
              deleting={deletingId === item.uuid}
              onRename={renamePasskey}
              onDelete={deletePasskey}
            />
          ))}
        </ul>
      )}
    </div>
  );
}
