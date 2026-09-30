import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import {
  wordmarkAccentEvenoddIndex,
  wordmarkAccentPaths,
  wordmarkGlyphPaths,
  wordmarkViewBox,
} from "./artwork";

type AppWordmarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/** Full wordmark lockup — glyph + accent (base artwork, to be replaced). */
export function AppWordmark({
  className,
  title = brand.productName,
  ...props
}: AppWordmarkProps) {
  return (
    <svg
      viewBox={wordmarkViewBox}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("h-9 w-auto", className)}
      {...props}
    >
      <title>{title}</title>
      {wordmarkGlyphPaths.map((d) => (
        <path key={d.slice(0, 24)} className="fill-brand-glyph" d={d} />
      ))}
      {wordmarkAccentPaths.map((d, i) => (
        <path
          key={d.slice(0, 24)}
          className="fill-brand-accent"
          fillRule={i === wordmarkAccentEvenoddIndex ? "evenodd" : undefined}
          d={d}
        />
      ))}
    </svg>
  );
}
