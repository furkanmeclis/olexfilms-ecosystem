import { formatPlate } from "@/features/geo/lib/plate";
import type { PlateFormat } from "@/features/geo/types";
import { cn } from "@/lib/utils";

type PlateBadgeProps = {
  plate: string;
  format: Pick<
    PlateFormat,
    | "country_label"
    | "strip_color"
    | "background_color"
    | "text_color"
    | "input_mask"
  >;
  className?: string;
};

/**
 * Licence plate preview driven by the plate format (strip color, country
 * label, background). A physical plate reads left to right in every UI
 * direction, so the badge pins dir="ltr".
 */
export function PlateBadge({ plate, format, className }: PlateBadgeProps) {
  return (
    <span
      dir="ltr"
      className={cn(
        "inline-flex h-9 items-stretch overflow-hidden rounded-md border-2 border-black/70 font-mono text-lg leading-none font-semibold tracking-wider shadow-sm",
        className,
      )}
      style={{
        backgroundColor: format.background_color,
        color: format.text_color,
      }}
    >
      <span
        className="flex w-7 items-end justify-center pb-1 text-[10px] font-bold text-white"
        style={{ backgroundColor: format.strip_color }}
      >
        {format.country_label}
      </span>
      <span className="flex items-center px-3">
        {formatPlate(plate, format.input_mask) || " "}
      </span>
    </span>
  );
}
