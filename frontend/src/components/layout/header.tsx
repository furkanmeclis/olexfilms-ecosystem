"use client";

import {
  useEffect,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { UserMenu } from "@/components/layout/user-menu";
import { SearchTrigger } from "@/features/search-engine";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { NotificationInbox } from "@/features/notifications";
import { cn } from "@/lib/utils";

type HeaderProps = HTMLAttributes<HTMLElement> & {
  fixed?: boolean;
  children?: ReactNode;
};

export function Header({
  className,
  fixed = true,
  children,
  ...props
}: HeaderProps) {
  const [offset, setOffset] = useState(0);

  useEffect(() => {
    const onScroll = () => {
      setOffset(document.body.scrollTop || document.documentElement.scrollTop);
    };
    document.addEventListener("scroll", onScroll, { passive: true });
    return () => document.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header
      className={cn(
        "z-50 h-16",
        fixed && "header-fixed peer/header sticky top-0 w-[inherit]",
        offset > 10 && fixed ? "shadow" : "shadow-none",
        className,
      )}
      {...props}
    >
      <div
        className={cn(
          "relative flex h-full items-center gap-3 p-4 sm:gap-4",
          offset > 10 &&
            fixed &&
            "after:bg-background/20 after:absolute after:inset-0 after:-z-10 after:backdrop-blur-lg",
        )}
      >
        <SidebarTrigger variant="outline" className="max-md:scale-125" />
        <Separator orientation="vertical" className="h-6" />
        <div className="flex min-w-0 flex-1 items-center">{children}</div>
        <div className="ms-auto flex items-center gap-1">
          <SearchTrigger compact className="md:hidden" />
          <NotificationInbox />
          <LocaleSwitch />
          <ThemeSwitch />
          <UserMenu />
        </div>
      </div>
    </header>
  );
}
