"use client";

import { Suspense } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { LoginForm } from "@/features/auth/components/login-form";
import { useLocale } from "@/providers/locale-provider";

function LoginContent() {
  const { t } = useLocale();

  return (
    <RouteGuard mode="guest">
      <AuthShell>
        <Suspense
          fallback={
            <div className="text-muted-foreground text-center text-sm">
              {t("common.loading")}
            </div>
          }
        >
          <LoginForm />
        </Suspense>
      </AuthShell>
    </RouteGuard>
  );
}

export default function LoginPage() {
  return <LoginContent />;
}
