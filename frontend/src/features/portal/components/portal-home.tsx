"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { AIConsentDialog } from "@/features/portal/components/ai-consent-dialog";
import { portalApi, portalSignOut } from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

/** Portal landing page (F1 fills it with services and warranties). */
export function PortalHome() {
  const { t } = useLocale();
  const router = useRouter();
  const [name, setName] = useState<string | null>(null);

  useEffect(() => {
    portalApi
      .me()
      .then((me) => {
        const full = [me.user?.name, me.user?.surname]
          .filter((v) => v && v.trim())
          .join(" ");
        setName(full || null);
      })
      .catch(() => undefined);
  }, []);

  const onLogout = async () => {
    // Portal only: the panel session (if any) stays signed in.
    await portalSignOut();
    router.replace(routes.portal.login);
    router.refresh();
  };

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between gap-4">
        <h1 className="text-xl font-semibold">{t("portal.home.title")}</h1>
        <Button variant="outline" onClick={() => void onLogout()}>
          {t("portal.home.logout")}
        </Button>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>
            {name
              ? t("portal.home.welcome", { name })
              : t("portal.home.welcome_anonymous")}
          </CardTitle>
          <CardDescription>{t("portal.home.empty")}</CardDescription>
        </CardHeader>
        <CardContent />
      </Card>
      <AIConsentDialog />
    </div>
  );
}
