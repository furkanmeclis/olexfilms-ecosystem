"use client";

import { RouteGuard } from "@/components/common/route-guard";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { ForgotPasswordForm } from "@/features/auth/components/forgot-password-form";

export default function ForgotPasswordPage() {
  return (
    <RouteGuard mode="guest">
      <AuthShell>
        <ForgotPasswordForm />
      </AuthShell>
    </RouteGuard>
  );
}
