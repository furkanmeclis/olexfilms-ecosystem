"use client";

import { Info } from "lucide-react";
import { Direction } from "radix-ui";
import type { ReactNode } from "react";

import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import { SidebarMenuAction } from "@/components/ui/sidebar";
import type { NavBadgeVariant, NavInfo } from "@/features/nav-engine/types";
import { cn } from "@/lib/utils";

const TONE_CLASS: Record<NavBadgeVariant, string> = {
  default: "text-foreground",
  secondary: "text-muted-foreground",
  success: "text-emerald-700 dark:text-emerald-300",
  warning: "text-amber-700 dark:text-amber-300",
  danger: "text-destructive",
  outline: "text-foreground",
};

export function hasNavInfo(info: NavInfo | null | undefined): boolean {
  if (!info) return false;
  return Boolean(
    info.render || info.title || info.description || info.rows?.length,
  );
}

export function NavInfoButton({
  info,
  label,
}: {
  info: NavInfo;
  label: string;
}) {
  const dir = Direction.useDirection();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <SidebarMenuAction
          aria-label={label}
          title={label}
          className="text-muted-foreground hover:text-sidebar-accent-foreground"
        >
          <Info />
        </SidebarMenuAction>
      </PopoverTrigger>
      <PopoverContent
        side={dir === "rtl" ? "left" : "right"}
        align="start"
        className="w-64 p-3"
      >
        <NavInfoBody info={info} />
      </PopoverContent>
    </Popover>
  );
}

export function NavInfoBody({ info }: { info: NavInfo }) {
  if (info.render) return <>{info.render()}</>;

  return (
    <div className="space-y-2">
      {info.title || info.description ? (
        <PopoverHeader>
          {info.title ? <PopoverTitle>{info.title}</PopoverTitle> : null}
          {info.description ? (
            <PopoverDescription>{info.description}</PopoverDescription>
          ) : null}
        </PopoverHeader>
      ) : null}
      {info.rows?.length ? <NavInfoRows rows={info.rows} /> : null}
    </div>
  );
}

export function NavInfoRows({
  rows,
  className,
}: {
  rows: NonNullable<NavInfo["rows"]>;
  className?: string;
}) {
  return (
    <dl className={cn("grid gap-1.5 text-xs", className)}>
      {rows.map((row) => (
        <div
          key={row.label}
          className="flex items-baseline justify-between gap-3"
        >
          <dt className="text-muted-foreground">{row.label}</dt>
          <dd
            className={cn(
              "font-medium tabular-nums",
              row.tone ? TONE_CLASS[row.tone] : "text-foreground",
            )}
          >
            {row.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export function NavInfoSummary({ info }: { info: NavInfo }) {
  if (info.render) return <div className="ps-6">{info.render()}</div>;
  if (!info.rows?.length) {
    return info.description ? (
      <span className="text-muted-foreground ps-6 text-[10px] leading-tight">
        {info.description}
      </span>
    ) : null;
  }

  return (
    <span className="text-muted-foreground ps-6 text-[10px] leading-tight">
      {info.rows
        .map((row) => `${row.label} ${stringifyInfoValue(row.value)}`)
        .join(" · ")}
    </span>
  );
}

function stringifyInfoValue(value: ReactNode): string {
  if (value == null || typeof value === "boolean") return "—";
  if (typeof value === "string" || typeof value === "number") {
    return String(value);
  }
  return "";
}
