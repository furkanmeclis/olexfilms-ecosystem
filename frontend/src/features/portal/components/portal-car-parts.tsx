"use client";

import { isKnownPart, partGroup } from "@/features/services/lib/car-parts";
import { CAR_SHAPES, CAR_VIEWBOX } from "@/features/services/lib/car-shapes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function fillClass(part: string, applied: boolean, highlighted: boolean) {
  const glass = partGroup(part) === "window";
  if (highlighted) return glass ? "fill-sky-500" : "fill-emerald-500";
  if (applied) return glass ? "fill-sky-200" : "fill-emerald-200";
  return glass ? "fill-[#bababa]" : "fill-[#efefef]";
}

/**
 * Read-only vehicle drawing (TEC-241) on the TEC-182 outline: the applied
 * parts are coloured, the highlighted ones (a focused product, or every
 * applied part) stronger. Static bundle only, no user SVG. The drawing
 * keeps its geometry in RTL (left / right are the vehicle's sides).
 */
export function PortalCarParts({
  applied,
  highlighted,
}: {
  applied: ReadonlySet<string>;
  highlighted: ReadonlySet<string>;
}) {
  const { t } = useLocale();
  const label = (part: string) =>
    isKnownPart(part) ? t(`services.parts.names.${part}`) : part;
  return (
    <div
      dir="ltr"
      className="bg-muted/30 rounded-lg border p-2"
      data-testid="portal-car-parts"
    >
      <svg
        viewBox={CAR_VIEWBOX}
        className="h-auto w-full"
        role="img"
        aria-label={t("portal.service.drawing_label")}
      >
        {CAR_SHAPES.map((shape) => {
          const part = shape.part;
          if (!part) {
            return (
              <path
                key={shape.id}
                d={shape.d}
                fillRule={shape.evenOdd ? "evenodd" : undefined}
                aria-hidden="true"
                className={cn(
                  "[stroke:#737373] [stroke-width:1.5]",
                  shape.tone === "glass" ? "fill-[#bababa]" : "fill-none",
                )}
              />
            );
          }
          const on = applied.has(part);
          const hot = highlighted.has(part);
          return (
            <path
              key={shape.id}
              d={shape.d}
              fillRule={shape.evenOdd ? "evenodd" : undefined}
              data-part={part}
              data-applied={on ? "true" : "false"}
              data-highlighted={hot ? "true" : "false"}
              className={cn(
                "[stroke:#737373] [stroke-width:1.5] transition-colors",
                fillClass(part, on, hot),
              )}
            >
              <title>{label(part)}</title>
            </path>
          );
        })}
      </svg>
    </div>
  );
}
