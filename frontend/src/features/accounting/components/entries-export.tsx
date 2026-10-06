"use client";

import { Download, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  EXPORT_POLL_MS,
  useAccountingExport,
} from "@/features/accounting/components/statement-export";
import {
  accountingService,
  STATEMENT_EXPORT_FORMATS,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Ledger list export (TEC-379 `POST /v1/accounting/entries/export`): the
 * toolbar menu queues the format with the list filters, q and sort; the
 * job is polled and the file downloaded like the statement export.
 */
export function EntriesExportMenu({
  orgUuid,
  query,
  pollMs = EXPORT_POLL_MS,
}: {
  orgUuid: string;
  query: Record<string, string>;
  pollMs?: number;
}) {
  const { t, locale } = useLocale();
  const { start, current, busy } = useAccountingExport({
    orgUuid,
    pollMs,
    request: (format) => accountingService.exportEntries(format, query, locale),
  });
  const label = t("exports.menu_label");
  const status = current
    ? t(`accounting.statement.export_status.${current.status}`)
    : null;

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-8"
              disabled={busy}
              aria-label={label}
              data-testid="entries-export"
              data-status={current?.status ?? "idle"}
            >
              {busy ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Download className="size-4" />
              )}
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{status ?? label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        {STATEMENT_EXPORT_FORMATS.map((format) => (
          <DropdownMenuItem
            key={format}
            disabled={busy}
            data-testid={`entries-export-${format}`}
            onClick={() => start.mutate(format)}
          >
            {t(`exports.formats.${format}`)}
          </DropdownMenuItem>
        ))}
        {current?.status === "completed" ? (
          <DropdownMenuItem
            onClick={() => void accountingService.downloadExport(current)}
          >
            {t("accounting.statement.download_again")}
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
