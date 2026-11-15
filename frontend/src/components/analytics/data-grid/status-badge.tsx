import { AlertTriangle, CheckCircle2, Circle, Clock } from "lucide-react";

import { cn } from "@/lib/utils";

import type { StatusCell, StatusTone } from "../types";

const TONE: Record<StatusTone, { color: string; Icon: typeof Circle }> = {
  neutral: { color: "var(--viz-muted)", Icon: Circle },
  progress: { color: "var(--viz-warning)", Icon: Clock },
  done: { color: "var(--viz-good)", Icon: CheckCircle2 },
  problem: { color: "var(--viz-critical)", Icon: AlertTriangle },
};

/** Status chip: icon + label (text in text colour), tinted by tone. */
export function StatusBadge({
  status,
  className,
}: {
  status: StatusCell;
  className?: string;
}) {
  const tone = TONE[status.tone] ?? TONE.neutral;
  const { Icon } = tone;
  return (
    <span
      className={cn(
        "text-foreground inline-flex max-w-full items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-medium",
        className,
      )}
      style={{
        background: `color-mix(in oklab, ${tone.color} 14%, transparent)`,
      }}
      data-slot="status-badge"
      data-tone={status.tone}
    >
      <Icon
        className="size-3.5 shrink-0"
        style={{ color: tone.color }}
        aria-hidden
      />
      <span className="truncate">{status.label}</span>
    </span>
  );
}
