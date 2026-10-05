import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import {
  artworkTransform,
  markAccentPaths,
  markDetailPath,
  markViewBox,
} from "./artwork";

type AppMarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/** Olex crest — gold mark with deep-green star details (same in both modes). */
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
      <g transform={artworkTransform}>
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
