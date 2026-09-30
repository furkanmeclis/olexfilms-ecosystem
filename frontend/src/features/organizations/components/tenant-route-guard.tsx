"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/organizations/providers/tenant-provider";
import { useAuthStore } from "@/lib/auth/session-store";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

export function TenantRouteGuard({
  children,
  mode,
}: {
  children: ReactNode;
  mode: "guest" | "tenant";
}) {
  const router = useRouter();
  const { slug, organization } = useTenant();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const { t } = useLocale();
  const cachedUser = useAuthStore((s) => s.user);

  const membership = user?.organizations.find((org) => org.slug === slug);
  const [nowMs] = useState(() => Date.now());
  const accessExpired =
    membership?.access_ends_at &&
    new Date(membership.access_ends_at).getTime() <= nowMs;

  useEffect(() => {
    if (!bootstrapped) return;
    if (mode === "guest" && isAuthenticated && membership && !accessExpired) {
      router.replace(routes.tenant.home(slug));
    }
    if (mode === "tenant" && !isAuthenticated) {
      router.replace(routes.tenant.login(slug));
    }
  }, [
    accessExpired,
    bootstrapped,
    isAuthenticated,
    membership,
    mode,
    router,
    slug,
  ]);

  // Show loading only when there is no cached user to render optimistically.
  // If cachedUser exists, render children immediately and let session
  // verification update state in the background.
  if (!bootstrapped && !cachedUser) {
    return (
      <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (mode === "tenant" && !isAuthenticated) {
    return null;
  }

  if (mode === "tenant" && isAuthenticated && !membership) {
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>{t("organizations.access.denied_title")}</CardTitle>
            <CardDescription>
              {t("organizations.access.denied_description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link href={routes.public.root}>
                {t("organizations.access.back_home")}
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (mode === "tenant" && accessExpired) {
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>{t("organizations.access.expired_title")}</CardTitle>
            <CardDescription>
              {t("organizations.access.expired_description", {
                name: organization?.name ?? slug,
              })}
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  return children;
}
