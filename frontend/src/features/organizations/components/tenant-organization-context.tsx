"use client";

import { useSession } from "next-auth/react";
import { useEffect, useRef, useState, type ReactNode } from "react";

import {
  endOrgSwitch,
  useOrgSwitchTarget,
} from "@/features/organizations/lib/org-switch";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

/**
 * Ensures the BFF session JWT carries organization scope (`oid`) for tenant APIs.
 * Skips the switch when the session already carries this slug's organization UUID
 * (typical after login-with-slug or oid-preserving refresh).
 *
 * While the header switcher moves the session to another organization
 * (TEC-227), a context for a different slug pauses its sync so it cannot
 * scope the session back; the target slug's context ends the switch.
 */
export function TenantOrganizationContext({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const { data: session, update, status: sessionStatus } = useSession();
  const updateRef = useRef(update);
  useEffect(() => {
    updateRef.current = update;
  });

  const { bootstrapped, isAuthenticated, user } = useAuth();
  const [ready, setReady] = useState(false);
  const switchTarget = useOrgSwitchTarget();
  const switchingAway = switchTarget !== null && switchTarget !== slug;

  const membership = user?.organizations.find((org) => org.slug === slug);
  const hasMembership = Boolean(membership);
  const sessionOrgUuid =
    (session as { organizationUuid?: string | null } | null)
      ?.organizationUuid ?? null;
  const alreadyScoped =
    Boolean(membership?.uuid) &&
    Boolean(sessionOrgUuid) &&
    membership?.uuid === sessionOrgUuid;

  useEffect(() => {
    let cancelled = false;

    (async () => {
      // Another organization is taking over: never pull the session back.
      if (switchingAway) return;
      if (!bootstrapped) return;
      if (sessionStatus === "loading") return;

      // Landing on the target organization ends a pending switch (no-op
      // for any other slug).
      const done = () => {
        endOrgSwitch(slug);
        if (!cancelled) setReady(true);
      };

      if (!isAuthenticated || !hasMembership || alreadyScoped) {
        done();
        return;
      }

      try {
        await authService.switchOrganizationContext(slug);
        await updateRef.current();
      } catch {
        // TenantRouteGuard / API errors surface access issues.
      } finally {
        done();
      }
    })();

    return () => {
      cancelled = true;
    };
    // update intentionally excluded: accessed via updateRef to avoid loops.
  }, [
    alreadyScoped,
    bootstrapped,
    hasMembership,
    isAuthenticated,
    sessionStatus,
    slug,
    switchingAway,
  ]);

  if (!ready) {
    return (
      <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  return children;
}
