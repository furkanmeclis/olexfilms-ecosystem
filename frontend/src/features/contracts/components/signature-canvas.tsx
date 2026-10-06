"use client";

import { Eraser } from "lucide-react";
import { useEffect, useRef, useState, type PointerEvent } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Point = { x: number; y: number };

/**
 * Canvas signature field (TEC-291): pointer events (mouse, pen and touch),
 * "Temizle" and a PNG data URL through `onChange` after every stroke (null
 * once cleared). The canvas keeps a light background so the exported PNG
 * reads the same in dark mode.
 */
export function SignatureCanvas({
  onChange,
  disabled = false,
  label,
  testId,
}: {
  onChange: (dataUrl: string | null) => void;
  disabled?: boolean;
  label: string;
  testId?: string;
}) {
  const { t } = useLocale();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const last = useRef<Point | null>(null);
  const [empty, setEmpty] = useState(true);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const ratio = window.devicePixelRatio || 1;
    if (rect.width > 0 && rect.height > 0) {
      canvas.width = Math.round(rect.width * ratio);
      canvas.height = Math.round(rect.height * ratio);
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.scale(ratio, ratio);
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.lineWidth = 2.5;
    ctx.strokeStyle = "#111827";
  }, []);

  const point = (e: PointerEvent<HTMLCanvasElement>): Point => {
    const rect = e.currentTarget.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  };

  const onDown = (e: PointerEvent<HTMLCanvasElement>) => {
    if (disabled) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture?.(e.pointerId);
    const p = point(e);
    last.current = p;
    const ctx = e.currentTarget.getContext("2d");
    if (ctx) {
      // A dot for a tap without movement.
      ctx.beginPath();
      ctx.moveTo(p.x, p.y);
      ctx.lineTo(p.x + 0.1, p.y + 0.1);
      ctx.stroke();
    }
  };

  const onMove = (e: PointerEvent<HTMLCanvasElement>) => {
    const from = last.current;
    if (!from || disabled) return;
    e.preventDefault();
    const to = point(e);
    const ctx = e.currentTarget.getContext("2d");
    if (ctx) {
      ctx.beginPath();
      ctx.moveTo(from.x, from.y);
      ctx.lineTo(to.x, to.y);
      ctx.stroke();
    }
    last.current = to;
  };

  const onUp = (e: PointerEvent<HTMLCanvasElement>) => {
    if (!last.current) return;
    last.current = null;
    e.currentTarget.releasePointerCapture?.(e.pointerId);
    setEmpty(false);
    onChange(e.currentTarget.toDataURL("image/png"));
  };

  const clear = () => {
    const canvas = canvasRef.current;
    canvas
      ?.getContext("2d")
      ?.clearRect(0, 0, canvas.width || 0, canvas.height || 0);
    last.current = null;
    setEmpty(true);
    onChange(null);
  };

  return (
    <div className="space-y-2" data-testid={testId}>
      <canvas
        ref={canvasRef}
        role="img"
        aria-label={label}
        data-testid="signature-canvas"
        data-empty={empty ? "true" : "false"}
        className={cn(
          "h-40 w-full touch-none rounded-md border border-dashed bg-white",
          disabled ? "cursor-not-allowed opacity-60" : "cursor-crosshair",
        )}
        onPointerDown={onDown}
        onPointerMove={onMove}
        onPointerUp={onUp}
        onPointerCancel={onUp}
      />
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground text-xs">
          {t("services.contract.signature_hint")}
        </span>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={disabled || empty}
          onClick={clear}
          data-testid="signature-clear"
        >
          <Eraser className="size-4" />
          {t("services.contract.clear")}
        </Button>
      </div>
    </div>
  );
}
