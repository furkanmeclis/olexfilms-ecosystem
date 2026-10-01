"use client";

import { useMutation } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import type { ReactNode } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { useFeature } from "@/features/modules/hooks/use-features";
import { moduleName } from "@/features/modules/lib/labels";
import { modulesService } from "@/features/modules/services/modules.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type FeatureGuardProps = {
  /** Tenant route slug (active organization). */
  slug: string;
  /** Module key from the backend catalog. */
  feature: string;
  children: ReactNode;
};

/**
 * Route guard for a module page: renders children only while the module is
 * on; otherwise a "module is off" screen with "Talep et". Pair it with the
 * nav entry's `feature` so a hidden menu item cannot be reached by URL.
 */
export function FeatureGuard({ slug, feature, children }: FeatureGuardProps) {
  const { t } = useLocale();
  const { enabled, isLoading } = useFeature(slug, feature);

  if (isLoading) return <Loading label={t("common.loading")} />;
  if (enabled) return <>{children}</>;
  return <FeatureDisabled feature={feature} />;
}

export function FeatureDisabled({ feature }: { feature: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRequest = can(permissions.modules.read);
  const request = useMutation({
    mutationFn: () => modulesService.request(feature),
    onSuccess: () => appToast.success(t("modules.requested")),
    onError: () => appToast.error(t("modules.request_failed")),
  });

  return (
    <div
      className="flex items-center justify-center p-6"
      data-testid="feature-disabled"
    >
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="size-4" aria-hidden />
            {t("modules.guard.title")}
          </CardTitle>
          <CardDescription>
            {t("modules.guard.description", {
              module: moduleName(t, feature),
            })}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-muted-foreground text-sm">
            {t("modules.guard.contact")}
          </p>
          {canRequest ? (
            <Button
              onClick={() => request.mutate()}
              disabled={request.isPending || request.isSuccess}
            >
              {t("modules.request")}
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
