"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  AI_USAGE_CHANNELS,
  AI_USAGE_POOLS,
  AI_USAGE_PURPOSES,
  aiUserName,
} from "@/features/ai-admin/lib/quota";
import type {
  AIUsageRow,
  AIUsageScope,
} from "@/features/ai-admin/services/ai-admin.service";
import { useLocale } from "@/providers/locale-provider";

export type FilterOption = { value: string; label: string };

const enumOptions = (values: readonly string[], prefix: string) =>
  values.map((value) => ({
    value,
    labelKey: `${prefix}.${value}`,
    label: value,
  }));

/**
 * Usage report columns (GET /v1/ai/usage, /v1/platform/ai/usage): sort
 * `created_at` | `tokens`; `created` date range, `tokens` number range,
 * CSV `channel`, `purpose`, `pool`, `model`, `user` and, on the platform
 * report, `organization`.
 */
export function useUsageColumns({
  scope,
  userOptions,
  organizationOptions,
  modelOptions,
}: {
  scope: AIUsageScope;
  userOptions: FilterOption[];
  organizationOptions: FilterOption[];
  modelOptions: FilterOption[];
}) {
  const { t, format } = useLocale();

  return useMemo(() => {
    const tokenCell = (value: number) => (
      <span className="text-sm tabular-nums">{format.number(value)}</span>
    );
    const columns = [
      createColumn<AIUsageRow>({
        accessorKey: "created_at",
        labelKey: "ai_admin.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      scope === "platform"
        ? createColumn<AIUsageRow>({
            id: "organization",
            accessorFn: (row) => row.organization.name,
            labelKey: "ai_admin.columns.organization",
            enableSorting: false,
            filterVariant: "faceted",
            param: "organization",
            filterOptions: organizationOptions,
            enableColumnFilter: organizationOptions.length > 0,
            cell: ({ row }) => (
              <span className="text-sm">{row.original.organization.name}</span>
            ),
          })
        : null,
      createColumn<AIUsageRow>({
        id: "user",
        accessorFn: (row) => aiUserName(row.user),
        labelKey: "ai_admin.columns.user",
        enableSorting: false,
        filterVariant: "faceted",
        param: "user",
        filterOptions: userOptions,
        enableColumnFilter: userOptions.length > 0,
        gridSecondary: true,
        cell: ({ row }) =>
          row.original.user ? (
            <span className="text-sm">{aiUserName(row.original.user)}</span>
          ) : (
            <span className="text-muted-foreground text-sm">
              {t("ai_admin.no_user")}
            </span>
          ),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "channel",
        labelKey: "ai_admin.columns.channel",
        enableSorting: false,
        filterVariant: "faceted",
        param: "channel",
        filterOptions: enumOptions(AI_USAGE_CHANNELS, "ai_admin.channel"),
        cell: ({ row }) => (
          <Badge variant="outline" className="font-normal">
            {t(`ai_admin.channel.${row.original.channel}`)}
          </Badge>
        ),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "purpose",
        labelKey: "ai_admin.columns.purpose",
        enableSorting: false,
        filterVariant: "faceted",
        param: "purpose",
        filterOptions: enumOptions(AI_USAGE_PURPOSES, "ai_admin.purpose"),
        cell: ({ row }) => (
          <span className="text-sm">
            {t(`ai_admin.purpose.${row.original.purpose}`)}
          </span>
        ),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "pool",
        labelKey: "ai_admin.columns.pool",
        enableSorting: false,
        filterVariant: "faceted",
        param: "pool",
        filterOptions: enumOptions(AI_USAGE_POOLS, "ai_admin.pool"),
        defaultHidden: scope === "tenant",
        cell: ({ row }) => (
          <span className="text-sm">
            {t(`ai_admin.pool.${row.original.pool}`)}
          </span>
        ),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "model",
        labelKey: "ai_admin.columns.model",
        enableSorting: false,
        filterVariant: "faceted",
        param: "model",
        filterOptions: modelOptions,
        enableColumnFilter: modelOptions.length > 0,
        cell: ({ row }) => (
          <code className="text-xs" dir="ltr">
            {row.original.model}
          </code>
        ),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "input_tokens",
        labelKey: "ai_admin.columns.input_tokens",
        enableSorting: false,
        enableColumnFilter: false,
        defaultHidden: true,
        cell: ({ row }) => tokenCell(row.original.input_tokens),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "output_tokens",
        labelKey: "ai_admin.columns.output_tokens",
        enableSorting: false,
        enableColumnFilter: false,
        defaultHidden: true,
        cell: ({ row }) => tokenCell(row.original.output_tokens),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "cache_read_tokens",
        labelKey: "ai_admin.columns.cache_read_tokens",
        enableSorting: false,
        enableColumnFilter: false,
        defaultHidden: true,
        cell: ({ row }) => tokenCell(row.original.cache_read_tokens),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "cache_write_tokens",
        labelKey: "ai_admin.columns.cache_write_tokens",
        enableSorting: false,
        enableColumnFilter: false,
        defaultHidden: true,
        cell: ({ row }) => tokenCell(row.original.cache_write_tokens),
      }),
      createColumn<AIUsageRow>({
        accessorKey: "tokens",
        labelKey: "ai_admin.columns.tokens",
        enableSorting: true,
        filterVariant: "number-range",
        param: "tokens",
        cell: ({ row }) => (
          <span className="text-sm font-medium tabular-nums">
            {format.number(row.original.tokens)}
          </span>
        ),
      }),
    ];
    return columns.filter(Boolean) as ColumnDef<AIUsageRow, unknown>[];
  }, [format, modelOptions, organizationOptions, scope, t, userOptions]);
}
