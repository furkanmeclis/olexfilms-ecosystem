import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { markAccentPath, markGlyphPath, markViewBox } from "./artwork";

type AppMarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/** OP lockup — terracotta P + themed O (white in dark, espresso in light). */
export function AppMark({
  className,
  title = brand.name,
  ...props
}: AppMarkProps) {
  return (
    <svg
      viewBox={markViewBox}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("size-8", className)}
      {...props}
    >
      <title>{title}</title>
      <path
        className="fill-brand-accent"
        fillRule="evenodd"
        d={markAccentPath}
      />
      <path className="fill-brand-glyph" d={markGlyphPath} />
    </svg>
  );
}
