"use client";

import { Suspense } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { VerifyEmailForm } from "@/features/auth/components/verify-email-form";
import { useLocale } from "@/providers/locale-provider";

function VerifyEmailContent() {
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
          <VerifyEmailForm />
        </Suspense>
      </AuthShell>
    </RouteGuard>
  );
}

export default function VerifyEmailPage() {
  return <VerifyEmailContent />;
}
