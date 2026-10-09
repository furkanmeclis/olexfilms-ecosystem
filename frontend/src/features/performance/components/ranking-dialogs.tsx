"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  numberValue,
  periodStartDate,
  TARGET_METRICS,
  TARGET_PERIOD_KINDS,
  type TargetMetric,
} from "@/features/performance/lib/performance";
import {
  performanceKeys,
  performanceService,
  type PerformanceRankingRow,
} from "@/features/performance/services/performance.service";
import {
  taskKeys,
  tasksService,
} from "@/features/tasks/services/tasks.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** "Set target": a network target for the row's organization. */
export function TargetDialog({
  row,
  period,
  onOpenChange,
}: {
  row: PerformanceRankingRow | null;
  period: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [metric, setMetric] = useState<TargetMetric>("services_count");
  const [periodKind, setPeriodKind] =
    useState<(typeof TARGET_PERIOD_KINDS)[number]>("monthly");
  const [value, setValue] = useState("");
  const [note, setNote] = useState("");
  const valid = (numberValue(value) ?? 0) > 0;

  const save = useMutation({
    mutationFn: () =>
      performanceService.createTarget({
        target_organization_uuid: row?.organization_uuid ?? "",
        metric,
        period_kind: periodKind,
        period_start: periodStartDate(period),
        value: value.trim(),
        currency: metric === "order_volume" ? (row?.currency ?? null) : null,
        note: note.trim() || null,
      }),
    onSuccess: () => {
      appToast.success(t("performance.target.saved"));
      void qc.invalidateQueries({ queryKey: performanceKeys.all });
      setValue("");
      setNote("");
      onOpenChange(false);
    },
    onError: () => appToast.error(t("performance.target.failed")),
  });

  return (
    <Dialog open={Boolean(row)} onOpenChange={onOpenChange}>
      <DialogContent data-testid="performance-target-dialog">
        <DialogHeader>
          <DialogTitle>{t("performance.target.title")}</DialogTitle>
          <DialogDescription>{row?.name ?? ""}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label>{t("performance.target.metric")}</Label>
            <Select
              value={metric}
              onValueChange={(v) => setMetric(v as TargetMetric)}
            >
              <SelectTrigger aria-label={t("performance.target.metric")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TARGET_METRICS.map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(`performance.metrics.${m}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label>{t("performance.target.period_kind")}</Label>
            <Select
              value={periodKind}
              onValueChange={(v) =>
                setPeriodKind(v as (typeof TARGET_PERIOD_KINDS)[number])
              }
            >
              <SelectTrigger aria-label={t("performance.target.period_kind")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TARGET_PERIOD_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`performance.target.period_kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="performance-target-value">
              {t("performance.target.value")}
            </Label>
            <Input
              id="performance-target-value"
              inputMode="decimal"
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="performance-target-note">
              {t("performance.target.note")}
            </Label>
            <Textarea
              id="performance-target-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!valid || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** "Open task" (center, manual): a center task about the row's organization. */
export function TaskDialog({
  row,
  onOpenChange,
}: {
  row: PerformanceRankingRow | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const shownTitle =
    title ||
    (row ? t("performance.task.default_title", { name: row.name ?? "" }) : "");

  const save = useMutation({
    mutationFn: () =>
      tasksService.create({
        subject_organization_uuid: row?.organization_uuid ?? "",
        title: shownTitle.trim(),
        description: description.trim(),
      }),
    onSuccess: () => {
      appToast.success(t("performance.task.saved"));
      void qc.invalidateQueries({ queryKey: taskKeys.all });
      setTitle("");
      setDescription("");
      onOpenChange(false);
    },
    onError: () => appToast.error(t("performance.task.failed")),
  });

  return (
    <Dialog open={Boolean(row)} onOpenChange={onOpenChange}>
      <DialogContent data-testid="performance-task-dialog">
        <DialogHeader>
          <DialogTitle>{t("performance.task.title")}</DialogTitle>
          <DialogDescription>{row?.name ?? ""}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="performance-task-title">
              {t("performance.task.task_title")}
            </Label>
            <Input
              id="performance-task-title"
              value={shownTitle}
              maxLength={200}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="performance-task-description">
              {t("performance.task.description")}
            </Label>
            <Textarea
              id="performance-task-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!shownTitle.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
