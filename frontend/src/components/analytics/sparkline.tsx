import { cn } from "@/lib/utils";

import { isNumber } from "./format";

export type SparklineProps = {
  values: readonly (number | null | undefined)[];
  width?: number;
  height?: number;
  /** Line colour (default: categorical slot 1). */
  color?: string;
  /** Light area under the line. */
  area?: boolean;
  className?: string;
  /** Accessible name; omit when the value is already said next to it. */
  label?: string;
};

/** Path of a sparkline (exported for tests). Gaps break the line. */
export function sparklinePath(
  values: readonly (number | null | undefined)[],
  width: number,
  height: number,
  pad = 2,
): { line: string; area: string; last: { x: number; y: number } | null } {
  const nums = values.filter(isNumber);
  if (nums.length === 0) return { line: "", area: "", last: null };
  const min = Math.min(...nums);
  const max = Math.max(...nums);
  const span = max - min || 1;
  const n = values.length;
  const x = (i: number) =>
    n === 1 ? width / 2 : pad + (i * (width - pad * 2)) / (n - 1);
  const y = (v: number) =>
    max === min
      ? height / 2
      : height - pad - ((v - min) / span) * (height - pad * 2);
  let line = "";
  let area = "";
  let open = false;
  let segStart = 0;
  let last: { x: number; y: number } | null = null;
  values.forEach((v, i) => {
    if (!isNumber(v)) {
      if (open)
        area += `L${x(i - 1).toFixed(1)},${height}L${x(segStart).toFixed(1)},${height}Z`;
      open = false;
      return;
    }
    const px = x(i).toFixed(1);
    const py = y(v).toFixed(1);
    if (!open) {
      line += `M${px},${py}`;
      area += `M${px},${py}`;
      segStart = i;
      open = true;
    } else {
      line += `L${px},${py}`;
      area += `L${px},${py}`;
    }
    last = { x: x(i), y: y(v) };
  });
  if (open)
    area += `L${x(n - 1).toFixed(1)},${height}L${x(segStart).toFixed(1)},${height}Z`;
  return { line, area, last };
}

/** Tiny trend line without axes (KPI tiles, grid cells). */
export function Sparkline({
  values,
  width = 96,
  height = 28,
  color = "var(--viz-cat-1)",
  area = true,
  className,
  label,
}: SparklineProps) {
  const { line, area: areaPath, last } = sparklinePath(values, width, height);
  if (!line) return null;
  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={cn("overflow-visible", className)}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      data-slot="sparkline"
    >
      {area ? <path d={areaPath} fill={color} fillOpacity={0.12} /> : null}
      <path
        d={line}
        fill="none"
        stroke={color}
        strokeWidth={2}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      {last ? <circle cx={last.x} cy={last.y} r={2.5} fill={color} /> : null}
    </svg>
  );
}
