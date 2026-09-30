"use client";

import type { ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityDrawer } from "@/components/entity";
import type { AppLog } from "@/features/logs/services/logs.service";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type LogDetailDrawerProps = {
  log: AppLog | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function DetailField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs font-medium">{label}</dt>
      <dd className="text-sm break-words">{value ?? "—"}</dd>
    </div>
  );
}

function levelTone(level: AppLog["level"]) {
  if (level === "error") return "danger" as const;
  if (level === "warn") return "warning" as const;
  return "default" as const;
}

export function LogDetailDrawer({
  log,
  open,
  onOpenChange,
}: LogDetailDrawerProps) {
  const { t, locale } = useLocale();

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={t("logs.detail_title")}
      description={t("logs.detail_description")}
      size="lg"
    >
      {log ? (
        <dl className="grid gap-4 sm:grid-cols-2">
          <DetailField
            label={t("logs.columns.level")}
            value={
              <StatusChip
                label={t(`logs.levels.${log.level}`)}
                tone={levelTone(log.level)}
              />
            }
          />
          <DetailField
            label={t("logs.columns.created_at")}
            value={datetime(log.created_at, "dd.MM.yyyy HH:mm:ss", locale)}
          />
          <DetailField label={t("logs.columns.source")} value={log.source} />
          <DetailField
            label={t("logs.columns.request_id")}
            value={log.request_id}
          />
          <div className="space-y-1 sm:col-span-2">
            <dt className="text-muted-foreground text-xs font-medium">
              {t("logs.columns.message")}
            </dt>
            <dd className="bg-muted/40 rounded-md p-3 font-mono text-sm break-all whitespace-pre-wrap">
              {log.message}
            </dd>
          </div>
          {Object.keys(log.attrs ?? {}).length > 0 ? (
            <div className="space-y-1 sm:col-span-2">
              <dt className="text-muted-foreground text-xs font-medium">
                {t("logs.columns.attrs")}
              </dt>
              <dd>
                <pre className="bg-muted/40 max-h-72 overflow-auto rounded-md p-3 text-xs">
                  {JSON.stringify(log.attrs, null, 2)}
                </pre>
              </dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </EntityDrawer>
  );
}
