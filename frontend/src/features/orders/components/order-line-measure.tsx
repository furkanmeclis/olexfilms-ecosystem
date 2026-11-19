"use client";

import { AlertTriangle, Minus, Package, Plus, Ruler } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  MAX_LINE_CREDITS,
  boxConversion,
  boxesOf,
  isCutLine,
  isMeterLine,
  lineMeasure,
  metersNumber,
  staleMeterLine,
  stepMeters,
  wholeBoxBetter,
  type LineError,
  type OrderFormLine,
  type SaleMeasure,
} from "@/features/orders/lib/form";
import { useLocale } from "@/providers/locale-provider";

/** The message of a line error (the minimum cut names its meters). */
export function lineErrorText(
  t: (key: string, params?: Record<string, string | number>) => string,
  err: LineError,
  line: OrderFormLine,
): string {
  return err === "below_min_cut"
    ? t("orders.form.errors.below_min_cut", { meters: line.min_cut ?? "" })
    : t(`orders.form.errors.${err}`);
}

/**
 * TEC-1239: the measure badge of an order line (box / meter). The measure
 * of a line never changes; to change it, remove the line and add the
 * product again with the other button.
 */
export function MeasureBadge({
  measure,
  className,
}: {
  measure: SaleMeasure;
  className?: string;
}) {
  const { t } = useLocale();
  return (
    <Badge
      variant={measure === "meter" ? "default" : "secondary"}
      className={className}
      data-testid="line-measure"
      data-measure={measure}
      title={t("orders.measure.fixed_hint")}
    >
      {measure === "meter" ? (
        <Ruler className="size-3" />
      ) : (
        <Package className="size-3" />
      )}
      {t(`orders.measure.${measure}`)}
    </Badge>
  );
}

/**
 * The amount field of an order form line: pieces (boxes) or meters. A
 * meter line steps by 0.5 m with -/+ and still takes a typed decimal.
 */
export function LineAmountField({
  id,
  line,
  invalid,
  onChange,
}: {
  id: string;
  line: OrderFormLine;
  invalid?: boolean;
  onChange: (amount: string) => void;
}) {
  const { t } = useLocale();
  const meter = isMeterLine(line);
  const input = (
    <Input
      id={id}
      name="amount"
      inputMode={meter ? "decimal" : "numeric"}
      dir="ltr"
      autoComplete="off"
      className={meter ? "text-center tabular-nums" : undefined}
      value={line.amount}
      placeholder={meter && line.min_cut ? String(line.min_cut) : undefined}
      aria-invalid={invalid || undefined}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        if (!meter) return;
        if (e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          onChange(stepMeters(line.amount, e.key === "ArrowUp" ? 1 : -1));
        }
      }}
    />
  );
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-xs">
        {meter
          ? t("orders.form.meters")
          : isCutLineProduct(line)
            ? t("orders.form.boxes")
            : t("orders.form.quantity")}
      </Label>
      {meter ? (
        <div className="flex items-center gap-1" dir="ltr">
          <Button
            type="button"
            size="icon"
            variant="outline"
            className="size-9 shrink-0"
            aria-label={t("orders.measure.step_down")}
            data-testid="meters-step-down"
            onClick={() => onChange(stepMeters(line.amount, -1))}
          >
            <Minus className="size-4" />
          </Button>
          {input}
          <Button
            type="button"
            size="icon"
            variant="outline"
            className="size-9 shrink-0"
            aria-label={t("orders.measure.step_up")}
            data-testid="meters-step-up"
            onClick={() => onChange(stepMeters(line.amount, 1))}
          >
            <Plus className="size-4" />
          </Button>
        </div>
      ) : (
        input
      )}
    </div>
  );
}

/** A box line of a product also sold by the meter: "Boxes" not "Pieces". */
function isCutLineProduct(line: OrderFormLine): boolean {
  return lineMeasure(line) === "box" && line.sellable_by_meter === true;
}

/**
 * TEC-1239: the meter details of a cut line (a piece product sold by the
 * meter): the minimum cut hint, "≈ X boxes", the price of the meters, the
 * usage credits (default 5, K3), the "a whole box is better" warning with
 * the one-click conversion, and the warning of a kept draft line whose
 * product is no longer sold by the meter (K1).
 */
export function CutLineDetails({
  id,
  line,
  creditsInvalid,
  meterPrice,
  onCredits,
  onConvert,
}: {
  id: string;
  line: OrderFormLine;
  creditsInvalid?: boolean;
  /** Price of one meter (formatted) and of the typed meters; null = none. */
  meterPrice?: { unit: string; total: string | null } | null;
  onCredits: (value: string) => void;
  onConvert?: () => void;
}) {
  const { t, format } = useLocale();
  if (!isCutLine(line)) return null;
  const meters = metersNumber(line.amount);
  const boxes = boxesOf(meters, line.box_meters);
  const conv = wholeBoxBetter(line) ? boxConversion(line) : null;
  const num = (v: number) => format.number(v, { maximumFractionDigits: 2 });
  return (
    <div className="space-y-2 text-xs" data-testid="cut-line-details">
      <div className="text-muted-foreground flex flex-wrap gap-x-3 gap-y-1">
        {line.min_cut ? (
          <span data-testid="line-min-cut">
            {t("orders.measure.min_cut", { meters: num(line.min_cut) })}
          </span>
        ) : null}
        {line.box_meters ? (
          <span data-testid="line-box-meters">
            {t("orders.measure.box_meters", { meters: num(line.box_meters) })}
          </span>
        ) : null}
        {boxes !== null && meters > 0 ? (
          <span className="text-foreground" data-testid="line-boxes">
            {t("orders.measure.approx_boxes", { boxes: num(boxes) })}
          </span>
        ) : null}
        {meterPrice ? (
          <span className="text-foreground" data-testid="line-meter-price">
            {meterPrice.total !== null
              ? t("orders.measure.meter_total", {
                  unit: meterPrice.unit,
                  total: meterPrice.total,
                })
              : t("orders.measure.meter_unit", { unit: meterPrice.unit })}
          </span>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        <Label htmlFor={`${id}-credits`} className="text-xs font-normal">
          {t("orders.measure.credits")}
        </Label>
        <Input
          id={`${id}-credits`}
          name="line_credits"
          inputMode="numeric"
          dir="ltr"
          autoComplete="off"
          maxLength={String(MAX_LINE_CREDITS).length}
          className="h-8 w-20"
          data-testid="line-credits"
          title={t("orders.measure.credits_hint")}
          value={line.line_credits ?? ""}
          aria-invalid={creditsInvalid || undefined}
          onChange={(e) => onCredits(e.target.value)}
        />
      </div>
      {conv && onConvert ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-2 rounded-md border border-amber-500/50 bg-amber-50 p-2 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200"
          data-testid="whole-box-warning"
        >
          <AlertTriangle className="size-4 shrink-0" />
          <span className="flex-1">{t("orders.measure.whole_box")}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            data-testid="whole-box-convert"
            onClick={onConvert}
          >
            {conv.rest > 0
              ? t("orders.measure.convert_with_rest", {
                  boxes: conv.boxes,
                  rest: num(conv.rest),
                })
              : t("orders.measure.convert", { boxes: conv.boxes })}
          </Button>
        </div>
      ) : null}
      {staleMeterLine(line) ? (
        <p
          role="alert"
          className="flex items-start gap-1 text-amber-700 dark:text-amber-400"
          data-testid="line-meter-stale"
        >
          <AlertTriangle className="mt-0.5 size-3 shrink-0" />
          {t("orders.measure.stale")}
        </p>
      ) : null}
    </div>
  );
}
