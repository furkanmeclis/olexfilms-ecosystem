"use client";

import { Suspense } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { RegisterForm } from "@/features/auth/components/register-form";
import { useLocale } from "@/providers/locale-provider";

function RegisterContent() {
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
          <RegisterForm />
        </Suspense>
      </AuthShell>
    </RouteGuard>
  );
}

export default function RegisterPage() {
  return <RegisterContent />;
}
