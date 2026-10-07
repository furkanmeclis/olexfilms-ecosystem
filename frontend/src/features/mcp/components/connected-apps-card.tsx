"use client";

import { Plug } from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ConnectedAppsTable } from "@/features/mcp/components/connected-apps-table";
import type { SessionRealm } from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";

/** Profile section "Connected apps" (TEC-403) of the panel or the portal. */
export function ConnectedAppsCard({ realm }: { realm: SessionRealm }) {
  const { t } = useLocale();
  return (
    <Card className="shadow-none" data-testid="connected-apps-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Plug className="text-muted-foreground size-5" aria-hidden />
          {t("mcp.grants.title")}
        </CardTitle>
        <CardDescription>{t("mcp.grants.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <ConnectedAppsTable realm={realm} />
      </CardContent>
    </Card>
  );
}
