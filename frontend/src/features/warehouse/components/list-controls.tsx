"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const ALL = "all";

/** Status filter chips of the slice-2 lists (transfers, counts). */
export function StatusFilter<S extends string>({
  value,
  statuses,
  labelKey,
  ariaLabel,
  onChange,
}: {
  value: S | typeof ALL;
  statuses: S[];
  /** i18n prefix of the status labels, e.g. `warehouse.transfer_status`. */
  labelKey: string;
  ariaLabel: string;
  onChange: (next: S | typeof ALL) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="flex flex-wrap gap-2" role="group" aria-label={ariaLabel}>
      {[ALL, ...statuses].map((s) => (
        <Button
          key={s}
          type="button"
          size="sm"
          variant={value === s ? "secondary" : "ghost"}
          aria-pressed={value === s}
          onClick={() => onChange(s as S | typeof ALL)}
        >
          {s === ALL
            ? t("warehouse.entries.all_statuses")
            : t(`${labelKey}.${s}`)}
        </Button>
      ))}
    </div>
  );
}

/** Previous / next pager with the "page x / y" summary. */
export function Pager({
  page,
  pages,
  total,
  busy,
  hidden,
  onPage,
}: {
  page: number;
  pages: number;
  total: number;
  busy?: boolean;
  hidden?: boolean;
  onPage: (next: number) => void;
}) {
  const { t } = useLocale();
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-2",
        hidden && "hidden",
      )}
    >
      <p className="text-muted-foreground text-sm">
        {t("warehouse.list.page", { page: page + 1, pages, total })}
      </p>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page === 0 || busy}
          onClick={() => onPage(Math.max(0, page - 1))}
        >
          <ChevronLeft className="size-4 rtl:rotate-180" />
          {t("warehouse.list.prev")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page + 1 >= pages || busy}
          onClick={() => onPage(page + 1)}
        >
          {t("warehouse.list.next")}
          <ChevronRight className="size-4 rtl:rotate-180" />
        </Button>
      </div>
    </div>
  );
}

/** Loading, error and empty states of a list; renders children otherwise. */
export function ListBody({
  isError,
  isLoading,
  isEmpty,
  onRetry,
  emptyTitle,
  emptyDescription,
  emptyTestId,
  children,
}: {
  isError: boolean;
  isLoading: boolean;
  isEmpty: boolean;
  onRetry: () => void;
  emptyTitle: string;
  emptyDescription?: string;
  emptyTestId?: string;
  children: React.ReactNode;
}) {
  const { t } = useLocale();
  if (isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("warehouse.list.loading")}
      </p>
    );
  }
  if (isEmpty) {
    return (
      <div className="py-8 text-center" data-testid={emptyTestId}>
        <p className="font-medium">{emptyTitle}</p>
        {emptyDescription ? (
          <p className="text-muted-foreground text-sm">{emptyDescription}</p>
        ) : null}
      </div>
    );
  }
  return <>{children}</>;
}

/** Label + value cell of the detail summary cards. */
export function Info({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <p className="text-muted-foreground text-xs">{label}</p>
      <div className="text-sm font-medium">{children}</div>
    </div>
  );
}
