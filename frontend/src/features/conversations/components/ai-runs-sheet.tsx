"use client";

import { Activity, AlertTriangle, Bot, Clock } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useConversationAIRuns } from "@/features/conversations/hooks/use-conversations";
import type { ConversationAIRun } from "@/features/conversations/services/conversations.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const RUN_TONE: Record<ConversationAIRun["status"], string> = {
  running: "border-sky-500/40 bg-sky-500/10 text-sky-700",
  completed: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700",
  failed: "border-destructive/40 bg-destructive/10 text-destructive",
  skipped: "border-muted-foreground/40 bg-muted text-muted-foreground",
};

function fmtDuration(ms: number | null | undefined) {
  if (ms === null || ms === undefined) return null;
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

function RunRow({ run }: { run: ConversationAIRun }) {
  const { t, format } = useLocale();
  const duration = fmtDuration(run.duration_ms);
  return (
    <li
      className="space-y-2 border-b py-3 last:border-b-0"
      data-testid="ai-run-row"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge
          variant="outline"
          className={cn("font-normal", RUN_TONE[run.status])}
        >
          {t(`conversations.ai_runs.status.${run.status}`)}
        </Badge>
        {run.model ? (
          <span className="text-sm font-medium">{run.model}</span>
        ) : null}
        <span className="text-muted-foreground ms-auto text-xs">
          {format.dateTime(run.created_at)}
        </span>
      </div>
      <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
        <span>{t("conversations.ai_runs.tokens", { count: run.tokens })}</span>
        {duration ? (
          <span>{t("conversations.ai_runs.duration", { duration })}</span>
        ) : null}
      </div>
      {run.error ? (
        <p className="text-destructive flex items-start gap-1 text-xs">
          <AlertTriangle className="mt-0.5 size-3 shrink-0" />
          <span className="break-words">{run.error}</span>
        </p>
      ) : null}
    </li>
  );
}

export function AIRunsSheet({
  conversationUuid,
}: {
  conversationUuid: string;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const runs = useConversationAIRuns(conversationUuid, open);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          data-testid="ai-runs-open"
        >
          <Bot className="size-4" />
          {t("conversations.ai_runs.action")}
        </Button>
      </SheetTrigger>
      <SheetContent className="flex w-full flex-col sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{t("conversations.ai_runs.title")}</SheetTitle>
          <SheetDescription>
            {t("conversations.ai_runs.description")}
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto pe-1">
          {runs.isLoading ? (
            <div className="text-muted-foreground flex items-center gap-2 py-6 text-sm">
              <Clock className="size-4 animate-pulse" />
              {t("common.loading")}
            </div>
          ) : runs.isError ? (
            <div className="text-destructive flex items-center gap-2 py-6 text-sm">
              <AlertTriangle className="size-4" />
              {t("common.error_generic")}
            </div>
          ) : runs.data?.items.length ? (
            <ul data-testid="ai-runs-list">
              {runs.data.items.map((run) => (
                <RunRow key={run.uuid} run={run} />
              ))}
            </ul>
          ) : (
            <div className="text-muted-foreground flex flex-col items-center justify-center gap-2 py-12 text-center text-sm">
              <Activity className="size-8" />
              {t("conversations.ai_runs.empty")}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
