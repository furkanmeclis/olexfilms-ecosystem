"use client";

import { useQueryClient } from "@tanstack/react-query";
import { Building2, Check, ChevronsUpDown } from "lucide-react";
import { useSession } from "next-auth/react";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";

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
import { useMyOrganizations } from "@/features/organizations/hooks/use-my-organizations";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

/**
 * Header organization switcher. Lists the caller's memberships for the
 * domain's brand; choosing one re-scopes the session (`oid`) and opens
 * the organization's panel.
 */
export function OrganizationSwitcher({ className }: { className?: string }) {
  const { t } = useLocale();
  const { isAuthenticated } = useAuth();
  const { update } = useSession();
  const router = useRouter();
  const queryClient = useQueryClient();
  const params = useParams<{ slug?: string | string[] }>();
  const activeSlug = typeof params?.slug === "string" ? params.slug : null;
  const { data: organizations = [] } = useMyOrganizations(isAuthenticated);
  const [pendingSlug, setPendingSlug] = useState<string | null>(null);

  if (!isAuthenticated || organizations.length === 0) return null;

  const active = organizations.find((org) => org.slug === activeSlug) ?? null;

  const select = async (slug: string) => {
    if (slug === activeSlug || pendingSlug) return;
    setPendingSlug(slug);
    try {
      await authService.switchOrganizationContext(slug);
      await update();
      // Tenant data from the previous organization must not be reused.
      router.push(routes.tenant.home(slug));
      void queryClient.invalidateQueries();
    } catch {
      toast.error(t("organizations.switcher.error"));
    } finally {
      setPendingSlug(null);
    }
  };

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={cn("max-w-56 gap-2", className)}
          aria-label={t("organizations.switcher.label")}
          data-testid="organization-switcher"
        >
          <Building2 className="size-4 shrink-0" />
          <span className="truncate max-sm:hidden">
            {active?.name ?? t("organizations.switcher.select")}
          </span>
          <ChevronsUpDown className="size-3.5 shrink-0 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-64">
        <DropdownMenuLabel>
          {t("organizations.switcher.label")}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {organizations.map((org) => {
          const meta = [
            org.type ? t(`organizations.types.${org.type}`) : null,
            t(`organizations.roles.${org.role}`),
            org.parent?.name ?? null,
          ].filter(Boolean);
          return (
            <DropdownMenuItem
              key={org.uuid}
              disabled={pendingSlug !== null}
              onClick={() => void select(org.slug)}
              data-testid={`organization-switcher-item-${org.slug}`}
            >
              <div className="flex min-w-0 flex-col">
                <span className="truncate">{org.name}</span>
                <span className="text-muted-foreground truncate text-xs">
                  {meta.join(" · ")}
                </span>
              </div>
              <Check
                size={14}
                className={cn(
                  "ms-auto shrink-0",
                  org.slug !== activeSlug && "invisible",
                )}
              />
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
