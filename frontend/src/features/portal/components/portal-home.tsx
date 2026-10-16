"use client";

import { useQuery } from "@tanstack/react-query";
import {
  CalendarClock,
  Car,
  ChevronRight,
  FileSignature,
  House,
  LogOut,
  ShieldCheck,
  Store,
  UserRound,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { AIConsentDialog } from "@/features/portal/components/ai-consent-dialog";
import { PortalFleetHome } from "@/features/portal/components/portal-fleet";
import {
  PortalLoading,
  PortalPage,
} from "@/features/portal/components/portal-page";
import { portalApi, portalSignOut } from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

type Shortcut = {
  href: string;
  icon: LucideIcon;
  titleKey: string;
  hintKey: string;
  testId?: string;
};

/** TEC-771: every portal section one tap away, in menu order. */
const SHORTCUTS: readonly Shortcut[] = [
  {
    href: routes.portal.vehicles,
    icon: Car,
    titleKey: "portal.nav.vehicles",
    hintKey: "portal.home.vehicles_hint",
    testId: "portal-vehicles-link",
  },
  {
    href: routes.portal.warranties,
    icon: ShieldCheck,
    titleKey: "warranty.portal.home_link",
    hintKey: "portal.home.warranties_hint",
    testId: "portal-warranties-link",
  },
  {
    href: routes.portal.appointments,
    icon: CalendarClock,
    titleKey: "portal.nav.appointments",
    hintKey: "portal.home.appointments_hint",
  },
  {
    href: routes.portal.dealers,
    icon: Store,
    titleKey: "portal.nav.dealers",
    hintKey: "portal.home.dealers_hint",
  },
  {
    href: routes.portal.contracts,
    icon: FileSignature,
    titleKey: "portal.nav.contracts",
    hintKey: "portal.profile.contracts_hint",
  },
  {
    href: routes.portal.profile,
    icon: UserRound,
    titleKey: "portal.nav.profile",
    hintKey: "portal.home.profile_hint",
  },
];

/** Portal landing page: greeting, booking shortcut and the sections. */
export function PortalHome() {
  const { t } = useLocale();
  const router = useRouter();
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const name = [me.data?.user?.name, me.data?.user?.surname]
    .filter((v) => v && v.trim())
    .join(" ");

  const onLogout = async () => {
    // Portal only: the panel session (if any) stays signed in.
    await portalSignOut();
    router.replace(routes.portal.login);
    router.refresh();
  };

  if (isPortalReadOnly(me.data?.roles)) return <PortalFleetHome />;

  return (
    <PortalPage
      title={t("portal.home.title")}
      icon={<House className="size-6" />}
      testId="portal-home-page"
      actions={
        <Button variant="outline" size="sm" onClick={() => void onLogout()}>
          <LogOut className="size-4 rtl:rotate-180" />
          {t("portal.home.logout")}
        </Button>
      }
    >
      {me.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void me.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : me.isPending ? (
        <PortalLoading label={t("common.loading")} />
      ) : (
        <>
          <Card>
            <CardContent className="flex flex-wrap items-center justify-between gap-4">
              <div className="min-w-0 space-y-1">
                <p className="text-lg font-semibold break-words">
                  {name
                    ? t("portal.home.welcome", { name })
                    : t("portal.home.welcome_anonymous")}
                </p>
                <p className="text-muted-foreground text-sm">
                  {t("portal.home.empty")}
                </p>
              </div>
              <Button asChild data-testid="portal-book-link">
                <Link href={routes.portal.newAppointment()}>
                  <CalendarClock className="size-4" />
                  {t("portal.home.book")}
                </Link>
              </Button>
            </CardContent>
          </Card>

          <ul
            className="grid gap-3 sm:grid-cols-2"
            data-testid="portal-shortcuts"
          >
            {SHORTCUTS.map(
              ({ href, icon: Icon, titleKey, hintKey, testId }) => (
                <li key={href}>
                  <Link
                    href={href}
                    data-testid={testId}
                    className="group focus-visible:ring-ring block h-full rounded-xl outline-none focus-visible:ring-2"
                  >
                    <Card className="group-hover:border-primary/50 h-full transition-colors">
                      <CardContent className="flex items-center gap-3">
                        <span className="bg-muted text-foreground inline-flex size-10 shrink-0 items-center justify-center rounded-lg">
                          <Icon className="size-5" aria-hidden />
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block font-medium">
                            {t(titleKey)}
                          </span>
                          <span className="text-muted-foreground block text-sm">
                            {t(hintKey)}
                          </span>
                        </span>
                        <ChevronRight className="text-muted-foreground size-4 shrink-0 rtl:rotate-180" />
                      </CardContent>
                    </Card>
                  </Link>
                </li>
              ),
            )}
          </ul>
          <AIConsentDialog />
        </>
      )}
    </PortalPage>
  );
}
