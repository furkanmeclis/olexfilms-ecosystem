"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useShellNav } from "@/components/layout/use-shell-nav";
import { routes } from "@/config/routes";
import { shellNavCatalogs, type ShellVariant } from "@/config/shell-nav";
import { navRouteAccess } from "@/features/nav-engine/lib/route-access";
import {
  defaultHomeForUser,
  isCmsUser,
  isPlatformUser,
} from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type RouteGuardProps = {
  children: ReactNode;
  mode: "guest" | "platform" | "cms";
};

function PageLoader({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
      {label}
    </div>
  );
}

function RoleMismatch({ expected, home }: { expected: string; home: string }) {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t("errors.forbidden_title")}</CardTitle>
          <CardDescription>{t("errors.role_mismatch")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-muted-foreground text-sm">
            {t("errors.area_requires", { area: expected })}
          </p>
          <Button asChild>
            <Link href={home}>{t("errors.continue")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

export function RouteGuard({ children, mode }: RouteGuardProps) {
  const router = useRouter();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const { t } = useLocale();

  useEffect(() => {
    if (!bootstrapped) return;

    if (mode === "guest" && isAuthenticated && user) {
      router.replace(defaultHomeForUser(user));
      return;
    }

    if ((mode === "platform" || mode === "cms") && !isAuthenticated) {
      router.replace(routes.guest.login);
    }
  }, [bootstrapped, mode, isAuthenticated, user, router]);

  if (!bootstrapped) {
    return <PageLoader label={t("common.loading")} />;
  }

  if ((mode === "platform" || mode === "cms") && !isAuthenticated) {
    return null;
  }

  if (mode === "platform" && user && !isPlatformUser(user)) {
    return (
      <RoleMismatch
        expected={t("layout.nav_platform")}
        home={isCmsUser(user) ? routes.cms.home : routes.errors.forbidden}
      />
    );
  }

  if (mode === "cms" && user && !isCmsUser(user)) {
    return (
      <RoleMismatch
        expected="CMS"
        home={
          isPlatformUser(user) ? routes.platform.home : routes.errors.forbidden
        }
      />
    );
  }

  return <>{children}</>;
}

/**
 * TEC-771: page level twin of the menu. A page owned by a nav entry (main
 * menu or settings) that the menu hides for this user (permission, org
 * type, member role, module) shows a 403 card instead of its content, so
 * a typed or bookmarked URL cannot open what the menu does not offer. The
 * API still enforces every rule; this keeps the UI honest.
 */
export function NavRouteGuard({
  variant,
  tenantSlug,
  children,
}: {
  variant: ShellVariant;
  tenantSlug?: string;
  children: ReactNode;
}) {
  const pathname = usePathname();
  const { t } = useLocale();
  const nav = useShellNav(variant, tenantSlug);
  const access = navRouteAccess(
    pathname ?? "/",
    shellNavCatalogs(nav),
    nav.access,
    nav.homeHref,
  );

  if (access === "pending" && !nav.featuresFailed) {
    return (
      <div className="text-muted-foreground flex flex-1 items-center justify-center py-16 text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (access === "denied") {
    return (
      <div
        className="flex flex-1 items-center justify-center py-16"
        data-testid="nav-route-forbidden"
      >
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>{t("errors.forbidden_title")}</CardTitle>
            <CardDescription>{t("errors.page_forbidden_body")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link href={nav.homeHref}>{t("errors.continue")}</Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <>{children}</>;
}
