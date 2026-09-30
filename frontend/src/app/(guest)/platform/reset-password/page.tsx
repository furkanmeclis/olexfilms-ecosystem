"use client";

import { Suspense } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { ResetPasswordForm } from "@/features/auth/components/reset-password-form";
import { useLocale } from "@/providers/locale-provider";

function ResetPasswordContent() {
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
          <ResetPasswordForm />
        </Suspense>
      </AuthShell>
    </RouteGuard>
  );
}

export default function ResetPasswordPage() {
  return <ResetPasswordContent />;
}
