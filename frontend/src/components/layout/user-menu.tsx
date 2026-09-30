"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LogOut, UserRound } from "lucide-react";
import { toast } from "sonner";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { routes } from "@/config/routes";
import { isPlatformUser, primaryOrganizationSlug } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

function initials(fullName?: string) {
  const parts = fullName?.trim().split(/\s+/).filter(Boolean) ?? [];
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return `${a}${b}`.toUpperCase() || "U";
}

function tenantSlugFromPath(pathname: string): string | null {
  const match = pathname.match(/^\/t\/([^/]+)/);
  return match?.[1] ?? null;
}

function resolveProfileHref(
  pathname: string,
  user: ReturnType<typeof useAuth>["user"],
) {
  if (isPlatformUser(user)) {
    return routes.platform.profile.root;
  }
  const slug =
    tenantSlugFromPath(pathname) ?? primaryOrganizationSlug(user) ?? null;
  if (slug) {
    return routes.tenant.profile.root(slug);
  }
  return routes.cms.profile.root;
}

function resolveLogoutHref(
  pathname: string,
  user: ReturnType<typeof useAuth>["user"],
) {
  const slug =
    tenantSlugFromPath(pathname) ?? primaryOrganizationSlug(user) ?? null;
  if (slug) {
    return routes.tenant.login(slug);
  }
  return routes.guest.login;
}

export function UserMenu() {
  const { t } = useLocale();
  const pathname = usePathname();
  const { user, isAuthenticated, logout } = useAuth();

  if (!isAuthenticated) return null;

  const profileHref = resolveProfileHref(pathname, user);

  const onLogout = async () => {
    const redirectTo = resolveLogoutHref(pathname, user);
    try {
      await logout();
    } finally {
      toast.success(t("auth.logout"));
      window.location.replace(redirectTo);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="rounded-full"
          aria-label={t("auth.profile")}
        >
          <Avatar className="size-8">
            <AvatarFallback className="text-xs">
              {initials(user?.fullName)}
            </AvatarFallback>
          </Avatar>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col gap-0.5">
            <span className="truncate text-sm font-medium">
              {user?.fullName || t("auth.profile")}
            </span>
            {user?.email ? (
              <span className="text-muted-foreground truncate text-xs">
                {user.email}
              </span>
            ) : null}
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href={profileHref}>
            <UserRound />
            {t("auth.profile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onLogout}>
          <LogOut />
          {t("auth.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
