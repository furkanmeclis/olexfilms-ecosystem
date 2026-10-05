import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import {
  artworkTransform,
  markAccentPaths,
  markDetailPath,
  wordmarkGlyphPaths,
  wordmarkViewBox,
} from "./artwork";

type AppWordmarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/**
 * Full Olex lockup — gold crest + "OLEX" letters. The letters use
 * `--brand-glyph`: white on dark surfaces, deep green on light ones.
 */
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
      <g transform={artworkTransform}>
        {wordmarkGlyphPaths.map((d) => (
          <path
            key={d.slice(0, 24)}
            className="fill-brand-glyph"
            fillRule="evenodd"
            d={d}
          />
        ))}
        {markAccentPaths.map((d) => (
          <path
            key={d.slice(0, 24)}
            className="fill-brand-accent"
            fillRule="evenodd"
            d={d}
          />
        ))}
        <path
          className="fill-brand-detail"
          fillRule="evenodd"
          d={markDetailPath}
        />
      </g>
    </svg>
  );
}
