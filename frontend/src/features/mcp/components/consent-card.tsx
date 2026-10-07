"use client";

import { Plug, ShieldAlert } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { OAuthConsent } from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";

export type ConsentDecision =
  | { decision: "approve"; organizationUuid: string | null }
  | { decision: "deny" };

/**
 * OAuth consent of an MCP client (TEC-403): who asks, where the browser
 * goes next, which endpoint and (panel endpoints) which organization the
 * connection is bound to, and how many tools it gets there. "Allow" stays
 * disabled until an organization is chosen; the customer endpoint is
 * always bound to the brand center, so it has no organization picker.
 */
export function ConsentCard({
  consent,
  pending,
  onDecide,
}: {
  consent: OAuthConsent;
  pending?: boolean;
  onDecide: (decision: ConsentDecision) => void;
}) {
  const { t, format } = useLocale();
  const customer = consent.realm === "customer";
  const [orgUuid, setOrgUuid] = useState<string | null>(null);
  const orgs = consent.organizations;
  const chosen = customer
    ? (orgs[0] ?? null)
    : (orgs.find((o) => o.uuid === orgUuid) ?? null);
  const canApprove = !pending && chosen !== null;

  return (
    <Card className="w-full max-w-lg" data-testid="oauth-consent">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Plug className="text-muted-foreground size-5" aria-hidden />
          {t("mcp.consent.title", { client: consent.client_name })}
        </CardTitle>
        <CardDescription>
          {customer
            ? t("mcp.consent.description_customer")
            : t("mcp.consent.description_panel")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt className="text-muted-foreground">{t("mcp.consent.client")}</dt>
          <dd className="min-w-0">
            <span className="font-medium">{consent.client_name}</span>
            <span
              className="text-muted-foreground block truncate font-mono text-xs"
              dir="ltr"
            >
              {consent.client_id}
            </span>
          </dd>
          <dt className="text-muted-foreground">{t("mcp.consent.endpoint")}</dt>
          <dd className="min-w-0 space-y-1">
            <Badge variant="outline" className="font-normal">
              {t(`mcp.realm.${consent.realm}`)}
            </Badge>
            <span className="block font-mono text-xs break-all" dir="ltr">
              {consent.resource_url}
            </span>
          </dd>
        </dl>

        <Alert data-testid="oauth-consent-redirect">
          <ShieldAlert aria-hidden />
          <AlertTitle>{t("mcp.consent.redirect_title")}</AlertTitle>
          <AlertDescription>
            <span>
              {t("mcp.consent.redirect_body", { host: consent.redirect_host })}
            </span>
          </AlertDescription>
        </Alert>

        {orgs.length === 0 ? (
          <Alert variant="destructive" data-testid="oauth-consent-no-org">
            <AlertDescription>
              {customer
                ? t("mcp.consent.no_center")
                : t("mcp.consent.no_organization")}
            </AlertDescription>
          </Alert>
        ) : customer ? null : (
          <div className="space-y-2" data-testid="oauth-consent-orgs">
            <Label>{t("mcp.consent.organization")}</Label>
            <p className="text-muted-foreground text-xs">
              {t("mcp.consent.organization_hint")}
            </p>
            <RadioGroup
              value={orgUuid ?? ""}
              onValueChange={setOrgUuid}
              className="gap-2"
            >
              {orgs.map((org) => {
                const id = `oauth-org-${org.uuid}`;
                return (
                  <div
                    key={org.uuid}
                    className="flex items-center gap-3 rounded-md border p-3"
                  >
                    <RadioGroupItem value={org.uuid} id={id} />
                    <Label htmlFor={id} className="flex-1 font-normal">
                      <span className="font-medium">{org.name}</span>
                      <span className="text-muted-foreground ms-2 text-xs">
                        {t(`mcp.org_type.${org.type}`)}
                      </span>
                    </Label>
                  </div>
                );
              })}
            </RadioGroup>
          </div>
        )}

        {chosen ? (
          <p className="text-sm" data-testid="oauth-consent-summary">
            {chosen.tool_count != null
              ? t("mcp.consent.summary_count", {
                  count: format.number(chosen.tool_count),
                  organization: chosen.name,
                })
              : t("mcp.consent.summary", { organization: chosen.name })}{" "}
            {customer ? null : t("mcp.consent.summary_writes")}
          </p>
        ) : null}
      </CardContent>
      <CardFooter className="flex flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          disabled={pending}
          onClick={() => onDecide({ decision: "deny" })}
          data-testid="oauth-consent-deny"
        >
          {t("mcp.consent.deny")}
        </Button>
        <Button
          disabled={!canApprove}
          onClick={() =>
            onDecide({
              decision: "approve",
              organizationUuid: customer ? null : (chosen?.uuid ?? null),
            })
          }
          data-testid="oauth-consent-approve"
        >
          {t("mcp.consent.approve")}
        </Button>
      </CardFooter>
    </Card>
  );
}
