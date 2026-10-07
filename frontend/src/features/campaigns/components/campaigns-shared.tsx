"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Copy, Eye, OctagonX, Trash2 } from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CAMPAIGN_CHANNELS,
  CAMPAIGN_RECIPIENT_STATUSES,
  CAMPAIGN_STATUSES,
  campaignStats,
  campaignStatusTone,
  canCancel,
  recipientStatusTone,
} from "@/features/campaigns/lib/campaigns";
import type {
  Campaign,
  CampaignChannel,
  CampaignRecipient,
} from "@/features/campaigns/services/campaigns.service";

type CampaignT = (
  key: string,
  values?: Record<string, string | number>,
) => string;

export const CAMPAIGNS_PERSIST_KEY = "tenant-campaigns-v1";
export const CAMPAIGN_APPROVALS_PERSIST_KEY = "tenant-campaign-approvals-v1";
export const CAMPAIGN_RECIPIENTS_PERSIST_KEY = "tenant-campaign-recipients-v1";

export function campaignOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

export function ChannelBadges({
  channels,
  t,
}: {
  channels: readonly CampaignChannel[];
  t: CampaignT;
}) {
  return (
    <div className="flex flex-wrap gap-1">
      {channels.map((channel) => (
        <Badge key={channel} variant="outline">
          {t(`campaigns.channel.${channel}`)}
        </Badge>
      ))}
    </div>
  );
}

function dateOrDash(
  value: string | null,
  format: { dateTime: (v: string) => string },
) {
  return value ? format.dateTime(value) : "—";
}

export function campaignColumns({
  slug,
  t,
  format,
  onDuplicate,
  onCancel,
  onDelete,
}: {
  slug: string;
  t: CampaignT;
  format: { dateTime: (v: string) => string };
  onDuplicate?: (campaign: Campaign) => void;
  onCancel?: (campaign: Campaign) => void;
  onDelete?: (campaign: Campaign) => void;
}): ColumnDef<Campaign>[] {
  return [
    createColumn<Campaign>({
      accessorKey: "name",
      labelKey: "campaigns.columns.name",
      enableSorting: true,
      enableHiding: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <Link
          href={routes.tenant.campaigns.detail(slug, row.original.uuid)}
          className="font-medium hover:underline"
          onClick={(event) => event.stopPropagation()}
        >
          {row.original.name}
        </Link>
      ),
    }),
    createColumn<Campaign>({
      id: "channel",
      accessorFn: (row) => row.channels.join(","),
      labelKey: "campaigns.columns.channels",
      enableSorting: false,
      filterVariant: "faceted",
      filterOptions: campaignOptions(CAMPAIGN_CHANNELS, "campaigns.channel"),
      param: "channel",
      cell: ({ row }) => (
        <ChannelBadges channels={row.original.channels} t={t} />
      ),
    }),
    createColumn<Campaign>({
      accessorKey: "status",
      labelKey: "campaigns.columns.status",
      enableSorting: true,
      filterVariant: "faceted",
      filterOptions: campaignOptions(CAMPAIGN_STATUSES, "campaigns.status"),
      param: "status",
      cell: ({ row }) => (
        <StatusChip
          label={t(`campaigns.status.${row.original.status}`)}
          tone={campaignStatusTone(row.original.status)}
        />
      ),
    }),
    createColumn<Campaign>({
      accessorKey: "scheduled_at",
      labelKey: "campaigns.columns.scheduled_at",
      enableSorting: true,
      filterVariant: "date-range",
      param: "scheduled",
      cell: ({ row }) => dateOrDash(row.original.scheduled_at, format),
    }),
    createColumn<Campaign>({
      accessorKey: "recipients_total",
      labelKey: "campaigns.columns.recipients_total",
      enableSorting: false,
      cell: ({ row }) => row.original.recipients_total.toLocaleString(),
    }),
    createColumn<Campaign>({
      accessorKey: "recipients_sent",
      labelKey: "campaigns.columns.recipients_sent",
      enableSorting: false,
      cell: ({ row }) => row.original.recipients_sent.toLocaleString(),
    }),
    createColumn<Campaign>({
      accessorKey: "recipients_failed",
      labelKey: "campaigns.columns.recipients_failed",
      enableSorting: false,
      cell: ({ row }) => row.original.recipients_failed.toLocaleString(),
    }),
    createColumn<Campaign>({
      id: "actions",
      labelKey: "common.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () => {
                window.location.href = routes.tenant.campaigns.detail(
                  slug,
                  row.original.uuid,
                );
              },
            },
            {
              id: "duplicate",
              label: t("campaigns.actions.duplicate"),
              icon: Copy,
              permission: permissions.campaigns.write,
              onSelect: () => onDuplicate?.(row.original),
            },
            {
              id: "cancel",
              label: t("campaigns.actions.cancel"),
              icon: OctagonX,
              permission: permissions.campaigns.write,
              disabled: !canCancel(row.original.status),
              onSelect: () => onCancel?.(row.original),
            },
            {
              id: "delete",
              label: t("campaigns.actions.delete"),
              icon: Trash2,
              permission: permissions.campaigns.write,
              variant: "destructive",
              onSelect: () => onDelete?.(row.original),
            },
          ]}
        />
      ),
    }),
  ];
}

export function StatCards({ campaign }: { campaign: Campaign }) {
  const stats = campaignStats(campaign);
  const items = [
    ["campaigns.stats.total", campaign.recipients_total],
    ["campaigns.stats.sent", campaign.recipients_sent],
    ["campaigns.stats.failed", campaign.recipients_failed],
    ["campaigns.stats.skipped", campaign.recipients_skipped],
    ["campaigns.stats.pending", stats.pending],
  ] as const;
  return { items, successRate: stats.successRate };
}

export function recipientColumns({
  t,
  format,
}: {
  t: CampaignT;
  format: { dateTime: (v: string) => string };
}): ColumnDef<CampaignRecipient>[] {
  return [
    createColumn<CampaignRecipient>({
      accessorKey: "name",
      labelKey: "campaigns.recipients.name",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => row.original.name,
    }),
    createColumn<CampaignRecipient>({
      accessorKey: "channel",
      labelKey: "campaigns.columns.channels",
      enableSorting: true,
      filterVariant: "faceted",
      filterOptions: campaignOptions(CAMPAIGN_CHANNELS, "campaigns.channel"),
      param: "channel",
      cell: ({ row }) => t(`campaigns.channel.${row.original.channel}`),
    }),
    createColumn<CampaignRecipient>({
      accessorKey: "locale",
      labelKey: "campaigns.recipients.locale",
      enableSorting: true,
      filterVariant: "faceted",
      param: "locale",
      cell: ({ row }) => row.original.locale,
    }),
    createColumn<CampaignRecipient>({
      accessorKey: "status",
      labelKey: "campaigns.columns.status",
      enableSorting: true,
      filterVariant: "faceted",
      filterOptions: campaignOptions(
        CAMPAIGN_RECIPIENT_STATUSES,
        "campaigns.recipient_status",
      ),
      param: "status",
      cell: ({ row }) => (
        <StatusChip
          label={t(`campaigns.recipient_status.${row.original.status}`)}
          tone={recipientStatusTone(row.original.status)}
        />
      ),
    }),
    createColumn<CampaignRecipient>({
      accessorKey: "sent_at",
      labelKey: "campaigns.recipients.sent_at",
      enableSorting: true,
      filterVariant: "date-range",
      param: "sent",
      cell: ({ row }) => dateOrDash(row.original.sent_at, format),
    }),
  ];
}

export function CampaignBackButton({
  href,
  label,
}: {
  href: string;
  label: string;
}) {
  return (
    <Button asChild variant="outline" size="sm">
      <Link href={href}>{label}</Link>
    </Button>
  );
}
