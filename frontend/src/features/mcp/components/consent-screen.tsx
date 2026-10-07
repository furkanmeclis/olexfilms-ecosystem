"use client";

import { useQuery } from "@tanstack/react-query";
import { LogIn } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { AppWordmark } from "@/components/brand";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import {
  ConsentCard,
  type ConsentDecision,
} from "@/features/mcp/components/consent-card";
import {
  mcpService,
  type OAuthConsent,
  type SessionRealm,
} from "@/features/mcp/services/mcp.service";
import { useLocale } from "@/providers/locale-provider";

/** Path of the consent screen (Go `model.ConsentPath`). */
export const CONSENT_PATH = "/oauth/consent";

export function consentReturnPath(requestId: string) {
  return `${CONSENT_PATH}?request=${encodeURIComponent(requestId)}`;
}

type Loaded =
  | { kind: "ok"; realm: SessionRealm; consent: OAuthConsent }
  | { kind: "login" }
  | { kind: "missing" };

function statusOf(error: unknown): number {
  const status = (error as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status : 0;
}

/**
 * Loads the request with the session that may decide it: the panel
 * session for /mcp/dealer and /mcp/user, the customer portal session for
 * /mcp/customer. A session of the other realm answers 403, so the next
 * one is tried; without a fitting session the user signs in first and
 * comes back here.
 */
export async function loadConsent(
  requestId: string,
  sessions: { panel: boolean; portal: boolean },
): Promise<Loaded> {
  const order: SessionRealm[] = [];
  if (sessions.panel) order.push("panel");
  if (sessions.portal) order.push("portal");
  for (const realm of order) {
    try {
      return {
        kind: "ok",
        realm,
        consent: await mcpService.consent(realm, requestId),
      };
    } catch (error) {
      const status = statusOf(error);
      if (status === 404) return { kind: "missing" };
      if (status === 401 || status === 403) continue;
      throw error;
    }
  }
  return { kind: "login" };
}

export function ConsentScreen({
  requestId,
  sessions,
}: {
  requestId: string;
  sessions: { panel: boolean; portal: boolean };
}) {
  const { t } = useLocale();
  const [deciding, setDeciding] = useState(false);
  const [failed, setFailed] = useState(false);
  const query = useQuery({
    queryKey: ["mcp", "consent", requestId, sessions.panel, sessions.portal],
    queryFn: () => loadConsent(requestId, sessions),
    enabled: requestId !== "",
    retry: false,
    refetchOnWindowFocus: false,
  });

  const decide = async (realm: SessionRealm, d: ConsentDecision) => {
    setDeciding(true);
    setFailed(false);
    try {
      const out = await mcpService.decide(realm, requestId, {
        decision: d.decision,
        organization_uuid:
          d.decision === "approve" ? d.organizationUuid : undefined,
      });
      window.location.assign(out.redirect_url);
    } catch {
      setFailed(true);
      setDeciding(false);
    }
  };

  const back = consentReturnPath(requestId);
  let body: React.ReactNode;
  if (!requestId || query.data?.kind === "missing") {
    body = (
      <ErrorState
        title={t("mcp.consent.missing_title")}
        description={t("mcp.consent.missing_body")}
      />
    );
  } else if (query.isLoading) {
    body = <Loading label={t("common.loading")} />;
  } else if (query.isError) {
    body = (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (query.data?.kind === "login") {
    body = (
      <Card className="w-full max-w-lg" data-testid="oauth-consent-login">
        <CardHeader>
          <CardTitle>{t("mcp.consent.login_title")}</CardTitle>
          <CardDescription>{t("mcp.consent.login_body")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 sm:flex-row">
          <Button asChild>
            <Link
              href={`${routes.guest.login}?next=${encodeURIComponent(back)}`}
            >
              <LogIn aria-hidden />
              {t("mcp.consent.login_panel")}
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link
              href={`${routes.portal.login}?next=${encodeURIComponent(back)}`}
            >
              {t("mcp.consent.login_portal")}
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  } else if (query.data?.kind === "ok") {
    const { realm, consent } = query.data;
    body = (
      <div className="flex w-full flex-col items-center gap-3">
        <ConsentCard
          consent={consent}
          pending={deciding}
          onDecide={(d) => void decide(realm, d)}
        />
        {failed ? (
          <p className="text-destructive text-sm" role="alert">
            {t("mcp.consent.decide_failed")}
          </p>
        ) : null}
      </div>
    );
  }

  return (
    <main className="bg-muted/30 flex min-h-dvh flex-col items-center justify-center gap-6 p-4">
      <AppWordmark />
      {body}
    </main>
  );
}
