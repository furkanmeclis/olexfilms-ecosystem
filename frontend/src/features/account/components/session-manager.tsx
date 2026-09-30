"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Calendar,
  Clock,
  Loader2,
  Monitor,
  Shield,
  Trash2,
} from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { authService, type DeviceSession } from "@/services/auth.service";

function sessionLabel(item: DeviceSession, fallback: string) {
  const ua = item.user_agent?.trim();
  return ua && ua.length > 0 ? ua : fallback;
}

export function SessionManager() {
  const { t, locale } = useLocale();
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [revokingOthers, setRevokingOthers] = useState(false);
  const [pending, setPending] = useState<DeviceSession | null>(null);
  const [revokeOthersOpen, setRevokeOthersOpen] = useState(false);

  const sessionsQuery = useQuery({
    queryKey: ["account", "sessions"],
    queryFn: async () => {
      try {
        const list = await authService.listSessions();
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
  const items = sessionsQuery.data ?? [];
  const loading = sessionsQuery.isPending;

  const revoke = async (uuid: string) => {
    setRevokingId(uuid);
    try {
      await authService.revokeSession(uuid);
      toast.success(t("auth.sessions.revoke_success"));
      setPending(null);
      await sessionsQuery.refetch();
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      setRevokingId(null);
    }
  };

  const revokeOthers = async () => {
    setRevokingOthers(true);
    try {
      await authService.revokeOtherSessions();
      toast.success(t("auth.sessions.revoke_others_success"));
      setRevokeOthersOpen(false);
      await sessionsQuery.refetch();
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      setRevokingOthers(false);
    }
  };

  const othersExist = items.some((item) => !item.current);

  return (
    <div className="space-y-4">
      <Alert>
        <Shield className="size-4" aria-hidden />
        <AlertDescription>{t("auth.sessions.description")}</AlertDescription>
      </Alert>

      <div className="flex flex-wrap items-center justify-between gap-3">
        {loading ? (
          <Skeleton className="h-4 w-36" />
        ) : (
          <p className="text-muted-foreground text-sm">
            {items.length > 0
              ? t("auth.sessions.count", { count: items.length })
              : t("auth.sessions.empty_title")}
          </p>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={loading || !othersExist || revokingOthers}
          onClick={() => setRevokeOthersOpen(true)}
        >
          {revokingOthers ? (
            <Loader2 className="size-4 animate-spin" aria-hidden />
          ) : null}
          {t("auth.sessions.revoke_others")}
        </Button>
      </div>

      {loading ? (
        <ul className="space-y-3" aria-busy="true" aria-live="polite">
          <li className="border-border flex gap-4 rounded-xl border p-4">
            <Skeleton className="size-11 shrink-0 rounded-lg" />
            <div className="min-w-0 flex-1 space-y-3">
              <Skeleton className="h-4 w-48" />
              <Skeleton className="h-6 w-24" />
              <Skeleton className="h-3 w-full" />
            </div>
          </li>
        </ul>
      ) : items.length === 0 ? (
        <div className="border-border flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-10 text-center">
          <div className="bg-muted flex size-12 items-center justify-center rounded-full">
            <Monitor className="text-muted-foreground size-6" aria-hidden />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {t("auth.sessions.empty_title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("auth.sessions.empty_body")}
            </p>
          </div>
        </div>
      ) : (
        <ul className="space-y-3">
          {items.map((item) => (
            <li
              key={item.uuid}
              className="border-border bg-card flex flex-col gap-4 rounded-xl border p-4 shadow-xs sm:flex-row sm:items-start"
            >
              <div className="bg-primary/10 text-primary flex size-11 shrink-0 items-center justify-center rounded-lg">
                <Monitor className="size-5" aria-hidden />
              </div>
              <div className="min-w-0 flex-1 space-y-3">
                <div className="space-y-1">
                  <p className="text-sm font-medium break-all">
                    {sessionLabel(item, t("auth.sessions.unknown_device"))}
                  </p>
                  <div className="flex flex-wrap items-center gap-2">
                    {item.current ? (
                      <Badge variant="success">
                        {t("auth.sessions.current")}
                      </Badge>
                    ) : null}
                    {item.impersonated ? (
                      <Badge variant="outline">
                        {t("auth.sessions.impersonated")}
                      </Badge>
                    ) : null}
                    {item.ip_address ? (
                      <Badge variant="secondary">
                        {t("auth.sessions.ip")}: {item.ip_address}
                      </Badge>
                    ) : null}
                  </div>
                </div>
                <dl className="text-muted-foreground grid gap-2 text-xs sm:grid-cols-2">
                  <div className="flex items-center gap-2">
                    <Calendar className="size-3.5 shrink-0" aria-hidden />
                    <dd>
                      {t("auth.sessions.created_at")}:{" "}
                      {datetime(item.created_at, "dd MMM yyyy HH:mm", locale)}
                    </dd>
                  </div>
                  <div className="flex items-center gap-2">
                    <Clock className="size-3.5 shrink-0" aria-hidden />
                    <dd>
                      {t("auth.sessions.expires_at")}:{" "}
                      {datetime(item.expires_at, "dd MMM yyyy HH:mm", locale)}
                    </dd>
                  </div>
                </dl>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive shrink-0 self-start"
                disabled={revokingId === item.uuid}
                aria-label={t("auth.sessions.revoke")}
                onClick={() => setPending(item)}
              >
                {revokingId === item.uuid ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Trash2 className="size-4" aria-hidden />
                )}
              </Button>
            </li>
          ))}
        </ul>
      )}

      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("auth.sessions.revoke_title")}</DialogTitle>
            <DialogDescription>
              {t("auth.sessions.revoke_description")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setPending(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={!pending || revokingId === pending.uuid}
              onClick={() => {
                if (pending) void revoke(pending.uuid);
              }}
            >
              {revokingId
                ? t("auth.sessions.revoking")
                : t("auth.sessions.revoke")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={revokeOthersOpen} onOpenChange={setRevokeOthersOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("auth.sessions.revoke_others_title")}</DialogTitle>
            <DialogDescription>
              {t("auth.sessions.revoke_others_description")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRevokeOthersOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={revokingOthers}
              onClick={() => void revokeOthers()}
            >
              {revokingOthers
                ? t("auth.sessions.revoking")
                : t("auth.sessions.revoke_others")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
