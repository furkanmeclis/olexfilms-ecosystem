"use client";

import { useEffect, type ReactNode } from "react";
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
