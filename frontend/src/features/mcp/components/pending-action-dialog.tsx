"use client";

import { Clock } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

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
import {
  actionLabel,
  fieldLabel,
  warningLabel,
} from "@/features/ai-assistant/lib/labels";
import type { ActionPreview } from "@/features/ai-assistant/lib/types";
import type { AIActionCard } from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";

/** Seconds left until `expiresAt` (0 when past), ticking every second. */
export function useSecondsLeft(expiresAt: string | undefined) {
  const target = useMemo(
    () => (expiresAt ? new Date(expiresAt).getTime() : Number.NaN),
    [expiresAt],
  );
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (Number.isNaN(target)) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [target]);
  if (Number.isNaN(target)) return 0;
  return Math.max(0, Math.floor((target - now) / 1000));
}

export function formatCountdown(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/**
 * Confirmation card of a pending MCP / WhatsApp action (TEC-403, F4-01e
 * flow): what will be written, warnings and the time left. Confirm stays
 * disabled once the card expired; cancel resolves it without running.
 */
export function PendingActionDialog({
  action,
  pending,
  onConfirm,
  onCancel,
  onClose,
}: {
  action: AIActionCard | null;
  /** A decision request is in flight. */
  pending?: boolean;
  onConfirm: (action: AIActionCard) => void;
  onCancel: (action: AIActionCard) => void;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const secondsLeft = useSecondsLeft(action?.expires_at);
  const preview = (action?.preview ?? {}) as ActionPreview;
  const expired = !action || action.status !== "pending" || secondsLeft === 0;

  return (
    <Dialog open={action !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="pending-action-dialog">
        <DialogHeader>
          <DialogTitle>
            {action
              ? actionLabel(
                  t,
                  preview.action ?? action.tool_name,
                  preview.summary,
                )
              : null}
          </DialogTitle>
          <DialogDescription>
            {action ? format.dateTime(action.created_at) : null}
          </DialogDescription>
        </DialogHeader>

        {action ? (
          <div className="space-y-3">
            <Badge variant="outline">{t(`mcp.source.${action.source}`)}</Badge>
            {preview.summary ? (
              <p className="text-sm">{preview.summary}</p>
            ) : null}
            {preview.fields?.length ? (
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                {preview.fields.map((f, i) => (
                  <div key={`${f.key}-${i}`} className="contents">
                    <dt className="text-muted-foreground">
                      {fieldLabel(t, f.key)}
                    </dt>
                    <dd className="min-w-0 break-words">{f.value}</dd>
                  </div>
                ))}
              </dl>
            ) : null}
            {preview.warnings?.map((w) => {
              const label = warningLabel(t, w);
              return label ? (
                <Alert key={w}>
                  <AlertDescription>{label}</AlertDescription>
                </Alert>
              ) : null;
            })}
            {expired ? (
              <p
                className="text-destructive text-sm"
                data-testid="pending-action-expired"
              >
                {t("mcp.approvals.expired")}
              </p>
            ) : (
              <p
                className="text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums"
                data-testid="pending-action-countdown"
              >
                <Clock className="size-3" aria-hidden />
                {t("mcp.approvals.expires_in", {
                  time: formatCountdown(secondsLeft),
                })}
              </p>
            )}
          </div>
        ) : null}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => action && onCancel(action)}
            disabled={pending || !action || action.status !== "pending"}
            data-testid="pending-action-cancel"
          >
            {t("mcp.approvals.reject")}
          </Button>
          <Button
            onClick={() => action && onConfirm(action)}
            disabled={pending || expired}
            data-testid="pending-action-confirm"
          >
            {t("mcp.approvals.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
