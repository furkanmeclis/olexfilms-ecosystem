"use client";

import { Badge } from "@/components/ui/badge";
import type {
  GlorianOutboundState,
  GlorianSyncRunStatus,
} from "@/features/integrations/glorian/services/glorian.service";
import { useLocale } from "@/providers/locale-provider";

type Variant = "success" | "warning" | "danger" | "secondary" | "outline";

const RUN_VARIANT: Record<GlorianSyncRunStatus, Variant> = {
  running: "warning",
  succeeded: "success",
  failed: "danger",
};

const OUTBOUND_VARIANT: Record<GlorianOutboundState, Variant> = {
  pending: "secondary",
  held: "warning",
  sent: "success",
  failed: "danger",
  cancelled: "outline",
};

export function RunStatusBadge({ status }: { status: GlorianSyncRunStatus }) {
  const { t } = useLocale();
  return (
    <Badge variant={RUN_VARIANT[status] ?? "outline"}>
      {t(`integrations.glorian.run_status.${status}`)}
    </Badge>
  );
}

export function OutboundStateBadge({ state }: { state: GlorianOutboundState }) {
  const { t } = useLocale();
  return (
    <Badge variant={OUTBOUND_VARIANT[state] ?? "outline"}>
      {t(`integrations.glorian.outbound_state.${state}`)}
    </Badge>
  );
}

export const selectClass =
  "border-input bg-background h-9 rounded-md border px-3 text-sm";
