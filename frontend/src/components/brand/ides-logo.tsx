import { brand, type BrandTone, type BrandVariant } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";
import { AppWordmark } from "./app-wordmark";

type AppLogoProps = {
  variant?: BrandVariant;
  tone?: BrandTone;
  className?: string;
  width?: number;
  height?: number;
  priority?: boolean;
};

/**
 * App lockup. `mark` / `icon` → OP glyph; `logo` / `wordmark` → full wordmark.
 */
export function AppLogo({
  variant = "logo",
  className,
  width,
  height,
}: AppLogoProps) {
  const compact = variant === "mark" || variant === "icon";

  if (compact) {
    return (
      <AppMark
        width={width}
        height={height}
        className={cn("size-4 shrink-0", className)}
        title={brand.name}
      />
    );
  }

  return (
    <AppWordmark
      width={width}
      height={height}
      className={className}
      title={brand.productName}
    />
  );
}

/** @deprecated Prefer AppLogo — kept for layout import compatibility. */
export const IdesLogo = AppLogo;
