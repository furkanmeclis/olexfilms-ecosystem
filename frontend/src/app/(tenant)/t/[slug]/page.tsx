"use client";

import { PageHeader } from "@/components/layout";
import { useLocale } from "@/providers/locale-provider";

export default function TenantHomePage() {
  const { t } = useLocale();
  return (
    <PageHeader
      title={t("dashboard.tenant.welcome_title")}
      description={t("dashboard.tenant.welcome_description")}
    />
  );
}
