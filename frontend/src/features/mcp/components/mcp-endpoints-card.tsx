"use client";

import { Copy, Plug } from "lucide-react";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const MCP_FEATURE = "mcp";

const noop = () => () => {};

/** Public origin of the brand (the frontend proxies /mcp/* to Go). */
function useOrigin() {
  return useSyncExternalStore(
    noop,
    () => window.location.origin,
    () => "",
  );
}

/**
 * "MCP connection addresses" (TEC-403): the endpoint URLs this
 * organization's users paste into Claude, ChatGPT or another MCP client.
 * Shown only while the mcp module is on; /mcp/dealer only for dealer and
 * distributor organizations (docs/MCP.md).
 */
export function McpEndpointsCard({ slug }: { slug: string }) {
  const { t } = useLocale();
  const feature = useFeature(slug, MCP_FEATURE);
  const org = useActiveOrganization(slug);
  const origin = useOrigin();
  if (!feature.enabled || !origin) return null;

  const dealerOrg = org?.type === "dealer" || org?.type === "distributor";
  const endpoints = [
    ...(dealerOrg ? (["dealer"] as const) : []),
    "user" as const,
    "customer" as const,
  ];

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      appToast.success(t("mcp.endpoints.copied"));
    } catch {
      appToast.error(t("common.error_generic"));
    }
  };

  return (
    <Card className="shadow-none" data-testid="mcp-endpoints-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Plug className="text-muted-foreground size-5" aria-hidden />
          {t("mcp.endpoints.title")}
        </CardTitle>
        <CardDescription>{t("mcp.endpoints.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {endpoints.map((endpoint) => {
          const url = `${origin}/mcp/${endpoint}`;
          return (
            <div key={endpoint} className="space-y-1">
              <p className="text-sm font-medium">
                {t(`mcp.realm.${endpoint}`)}
              </p>
              <p className="text-muted-foreground text-xs">
                {t(`mcp.endpoints.hint_${endpoint}`)}
              </p>
              <div className="flex gap-2">
                <Input
                  readOnly
                  value={url}
                  dir="ltr"
                  className="font-mono text-xs"
                  aria-label={t(`mcp.realm.${endpoint}`)}
                  onFocus={(e) => e.currentTarget.select()}
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void copy(url)}
                  data-testid={`mcp-copy-${endpoint}`}
                >
                  <Copy aria-hidden />
                  {t("mcp.endpoints.copy")}
                </Button>
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
