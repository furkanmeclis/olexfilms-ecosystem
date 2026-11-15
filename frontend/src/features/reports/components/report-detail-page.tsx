"use client";

import { PageHeader } from "@/components/layout";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

/** TEC-1212: one report (filled once the analytics kit lands). */
export function ReportDetailPage({
  slug,
  reportKey,
}: {
  slug: string;
  reportKey: string;
}) {
  const { t } = useLocale();
  return (
    <PageHeader
      title={reportKey}
      breadcrumbs={[
        { label: t("reports.nav"), href: routes.tenant.reports.root(slug) },
        { label: reportKey },
      ]}
    />
  );
}
