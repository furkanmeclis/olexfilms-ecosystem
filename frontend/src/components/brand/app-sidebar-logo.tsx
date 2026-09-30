import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppSidebarLogoProps = {
  className?: string;
};

/** Sidebar header lockup — OP mark + product name. */
export function AppSidebarLogo({ className }: AppSidebarLogoProps) {
  return (
    <>
      <span
        className={cn(
          "flex aspect-square size-8 items-center justify-center",
          className,
        )}
      >
        <AppMark className="size-7" />
      </span>
      <span className="grid flex-1 text-left text-sm leading-tight">
        <span className="truncate font-medium">{brand.productName}</span>
        <span className="truncate text-xs opacity-70">{brand.subtitle}</span>
      </span>
    </>
  );
}
