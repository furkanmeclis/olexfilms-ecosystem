"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  cancelRequestStatusTone,
  subscriptionStatusTone,
} from "@/features/service-subscriptions/lib/subscriptions";
import type {
  CancelRequestStatus,
  ServiceSubscriptionStatus,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { useLocale } from "@/providers/locale-provider";

export function SubscriptionStatusChip({
  status,
}: {
  status: ServiceSubscriptionStatus;
}) {
  const { t } = useLocale();
  return (
    <span data-testid="subscription-status" data-status={status}>
      <StatusChip
        label={t(`catalog.subscriptions.status.${status}`)}
        tone={subscriptionStatusTone(status)}
      />
    </span>
  );
}

export function CancelRequestStatusChip({
  status,
}: {
  status: CancelRequestStatus;
}) {
  const { t } = useLocale();
  return (
    <span data-testid="cancel-request-status" data-status={status}>
      <StatusChip
        label={t(`catalog.subscriptions.request_status.${status}`)}
        tone={cancelRequestStatusTone(status)}
      />
    </span>
  );
}
