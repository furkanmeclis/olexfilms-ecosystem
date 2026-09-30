"use client";

import { LogOut } from "lucide-react";

import { Button } from "@/components/ui/button";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { appToast } from "@/providers/toast-provider";

export function ImpersonationBanner() {
  const { user, hydrateProfile } = useAuth();
  const { t } = useLocale();

  if (!user?.impersonation) return null;

  const stop = async () => {
    try {
      await authService.stopImpersonation();
      const refreshed = await hydrateProfile();
      appToast.success(t("users.toast.impersonation_stopped"));
      if (refreshed) {
        window.location.assign(defaultHomeForUser(refreshed));
      } else {
        window.location.reload();
      }
    } catch {
      appToast.error(t("common.error_generic"));
    }
  };

  return (
    <div className="mb-4 flex flex-col gap-3 rounded-lg border border-amber-500/30 bg-amber-500/15 px-4 py-3 text-amber-950 sm:flex-row sm:items-center sm:justify-between dark:text-amber-100">
      <p className="text-sm">
        {t("users.impersonation.banner", { name: user.fullName })}
      </p>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => void stop()}
      >
        <LogOut className="mr-2 size-4" />
        {t("users.impersonation.stop")}
      </Button>
    </div>
  );
}
