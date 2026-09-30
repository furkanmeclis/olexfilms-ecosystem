"use client";

import { Badge } from "@/components/ui/badge";
import type { StorageAccess } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

export function StorageAccessBadge({ access }: { access: StorageAccess }) {
  const { t } = useLocale();
  const variant =
    access === "public"
      ? "success"
      : access === "shared"
        ? "secondary"
        : access === "temporary"
          ? "warning"
          : access === "expired" || access === "revoked"
            ? "danger"
            : "outline";

  return (
    <Badge variant={variant}>
      {t(`storage.access_${access}` as "storage.access_private")}
    </Badge>
  );
}
