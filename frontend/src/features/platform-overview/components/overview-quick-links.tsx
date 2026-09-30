"use client";

import Link from "next/link";
import { Bell, Shield, UserRound, Users } from "lucide-react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type QuickLink = {
  title: string;
  description: string;
  href: string;
  icon: typeof Shield;
  visible: boolean;
};

export function OverviewQuickLinks() {
  const { t } = useLocale();
  const { can } = usePermission();

  const links: QuickLink[] = [
    {
      title: t("layout.nav_roles"),
      description: t("dashboard.link_roles_description"),
      href: routes.platform.roles.root,
      icon: Shield,
      visible: can(permissions.roles.read),
    },
    {
      title: t("layout.nav_users"),
      description: t("dashboard.link_users_description"),
      href: routes.platform.users.root,
      icon: Users,
      visible: can(permissions.users.read),
    },
    {
      title: t("layout.nav_notifications"),
      description: t("dashboard.link_notifications_description"),
      href: routes.platform.notifications.root,
      icon: Bell,
      visible: can(permissions.notifications.platformRead),
    },
    {
      title: t("layout.nav_profile"),
      description: t("dashboard.link_profile_description"),
      href: routes.platform.profile.root,
      icon: UserRound,
      visible: true,
    },
  ];

  const visible = links.filter((link) => link.visible);
  if (!visible.length) return null;

  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-base font-semibold tracking-tight">
          {t("dashboard.quick_links_title")}
        </h2>
        <p className="text-muted-foreground text-sm">
          {t("dashboard.quick_links_description")}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {visible.map((link) => {
          const Icon = link.icon;
          return (
            <Card
              key={link.href}
              className="hover:border-primary/40 hover:bg-muted/30 shadow-none transition-colors"
            >
              <Link
                href={link.href}
                className="block focus-visible:outline-none"
              >
                <CardHeader className="flex flex-row items-center gap-3 space-y-0 pb-1">
                  <Icon className="text-muted-foreground size-4 shrink-0" />
                  <CardTitle className="text-sm font-medium">
                    {link.title}
                  </CardTitle>
                </CardHeader>
                <CardContent className="pt-0">
                  <p className="text-muted-foreground text-sm">
                    {link.description}
                  </p>
                </CardContent>
              </Link>
            </Card>
          );
        })}
      </div>
    </section>
  );
}
