"use client";

import { CheckCircle2, Clock, ExternalLink, XCircle } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { actionLabel, fieldLabel, warningLabel } from "../lib/labels";
import type {
  ActionPreview,
  AIActionCard,
  AIActionOutcome,
} from "../lib/types";

/** Seconds left until `expiresAt` (0 when past), ticking every second. */
function useSecondsLeft(expiresAt: string, active: boolean) {
  const target = useMemo(() => new Date(expiresAt).getTime(), [expiresAt]);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [active]);
  if (Number.isNaN(target)) return 0;
  return Math.max(0, Math.floor((target - now) / 1000));
}

function formatCountdown(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/**
 * Confirmation card of a proposed write (TEC-387): preview rows, the
 * editable fields, warnings and a countdown to `expires_at`. Confirm sends
 * the edited values; nothing runs until the user confirms.
 */
export function ActionConfirmCard({
  card,
  disabled,
  canConfirm = true,
  onConfirm,
  onCancel,
}: {
  card: AIActionCard;
  disabled?: boolean;
  canConfirm?: boolean;
  onConfirm: (edits: Record<string, unknown> | undefined) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const preview = (card.preview ?? {}) as ActionPreview;
  const pending = card.status === "pending";
  const secondsLeft = useSecondsLeft(card.expires_at, pending);
  const expired = card.status === "expired" || (pending && secondsLeft === 0);

  const editable = useMemo(() => preview.edit ?? [], [preview.edit]);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(editable.map((f) => [f.key, f.value ?? ""])),
  );
  const missing = editable.some(
    (f) => f.required && !(values[f.key] ?? "").trim(),
  );

  const confirm = () => {
    if (editable.length === 0) return onConfirm(undefined);
    const edits: Record<string, unknown> = {};
    for (const f of editable) {
      const raw = values[f.key] ?? "";
      if (raw === (f.value ?? "")) continue;
      edits[f.key] = f.type === "number" && raw !== "" ? Number(raw) : raw;
    }
    onConfirm(Object.keys(edits).length ? edits : undefined);
  };

  const statusBadge = !pending ? (
    <Badge variant="outline" data-testid="ai-card-status">
      {t(`ai.card.status_${card.status}`)}
    </Badge>
  ) : null;

  return (
    <div
      className="bg-card space-y-3 rounded-lg border p-4 shadow-sm"
      data-testid="ai-confirm-card"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-semibold">
          {actionLabel(t, preview.action ?? card.tool_name, preview.summary)}
        </p>
        {statusBadge}
        {pending && !expired ? (
          <span
            className="text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums"
            data-testid="ai-card-countdown"
          >
            <Clock className="size-3" />
            {t("ai.card.expires_in", { time: formatCountdown(secondsLeft) })}
          </span>
        ) : null}
      </div>

      {/* TEC-461: the summary arrives in the user's language; it is the
          title itself when the action has no label. */}
      {preview.summary &&
      preview.summary !==
        actionLabel(t, preview.action ?? card.tool_name, preview.summary) ? (
        <p className="text-sm" data-testid="ai-card-summary">
          {preview.summary}
        </p>
      ) : null}

      {preview.fields?.length ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          {preview.fields.map((f, i) => (
            <div key={`${f.key}-${i}`} className="contents">
              <dt className="text-muted-foreground">{fieldLabel(t, f.key)}</dt>
              <dd className="min-w-0 break-words">{f.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {pending && !expired && editable.length ? (
        <div className="space-y-3 border-t pt-3">
          <p className="text-muted-foreground text-xs">
            {t("ai.card.edit_hint")}
          </p>
          {editable.map((f) => {
            const id = `ai-edit-${card.action_uuid}-${f.key}`;
            const value = values[f.key] ?? "";
            const set = (v: string) =>
              setValues((prev) => ({ ...prev, [f.key]: v }));
            return (
              <div key={f.key} className="space-y-1">
                <Label htmlFor={id}>
                  {fieldLabel(t, f.key)}
                  {f.required ? " *" : ""}
                </Label>
                {f.type === "textarea" ? (
                  <Textarea
                    id={id}
                    value={value}
                    onChange={(e) => set(e.target.value)}
                    rows={3}
                  />
                ) : f.type === "select" ? (
                  <select
                    id={id}
                    value={value}
                    onChange={(e) => set(e.target.value)}
                    className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
                  >
                    {(f.options ?? []).map((o) => (
                      <option key={o} value={o}>
                        {o}
                      </option>
                    ))}
                  </select>
                ) : (
                  <Input
                    id={id}
                    type={
                      f.type === "date" ||
                      f.type === "time" ||
                      f.type === "number"
                        ? f.type
                        : "text"
                    }
                    value={value}
                    onChange={(e) => set(e.target.value)}
                  />
                )}
              </div>
            );
          })}
        </div>
      ) : null}

      {preview.warnings?.map((w) => {
        const label = warningLabel(t, w);
        return label ? (
          <Alert key={w}>
            <AlertDescription>{label}</AlertDescription>
          </Alert>
        ) : null;
      })}

      {pending && expired ? (
        <p className="text-destructive text-sm">{t("ai.card.expired")}</p>
      ) : null}

      {pending && !expired && !canConfirm ? (
        <Alert data-testid="ai-card-confirm-permission">
          <AlertDescription>{t("ai.card.confirm_permission")}</AlertDescription>
        </Alert>
      ) : null}

      {pending && !expired && canConfirm ? (
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={onCancel}
            disabled={disabled}
            data-testid="ai-card-cancel"
          >
            {t("ai.card.cancel")}
          </Button>
          <Button
            size="sm"
            onClick={confirm}
            disabled={disabled || missing}
            data-testid="ai-card-confirm"
          >
            {t("ai.card.confirm")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

/** Result of a confirmed / cancelled action, with a link to the record. */
export function ActionOutcome({
  action,
  href,
}: {
  action: AIActionOutcome;
  href: string | null;
}) {
  const { t } = useLocale();
  const ok = action.status === "confirmed";
  const cancelled = action.status === "cancelled";
  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm",
        ok
          ? "border-emerald-500/40 bg-emerald-500/5"
          : cancelled
            ? "bg-muted/40"
            : "border-destructive/40 bg-destructive/5",
      )}
      data-testid="ai-action-outcome"
    >
      {ok ? (
        <CheckCircle2 className="size-4 text-emerald-600" />
      ) : (
        <XCircle
          className={cn(
            "size-4",
            cancelled ? "text-muted-foreground" : "text-destructive",
          )}
        />
      )}
      <span>
        {t(
          ok
            ? "ai.action.confirmed"
            : cancelled
              ? "ai.action.cancelled"
              : "ai.action.failed",
        )}
      </span>
      {href && ok ? (
        <Link
          href={href}
          className="text-primary ms-auto inline-flex items-center gap-1 underline-offset-2 hover:underline"
        >
          {t("ai.action.open_record")}
          <ExternalLink className="size-3" />
        </Link>
      ) : null}
    </div>
  );
}
