"use client";

import QRCode from "qrcode";
import { useEffect, useState } from "react";

import {
  PREVIEW_SAMPLE,
  qrPayload,
  qrSideMm,
  type LabelTemplateValues,
} from "@/features/stock/lib/label-templates";
import { useLocale } from "@/providers/locale-provider";

/** Preview width budget in CSS px; labels scale down to fit it. */
const PREVIEW_WIDTH_PX = 520;
/** CSS px per mm at 96 dpi. */
const PX_PER_MM = 96 / 25.4;
/** Gap between labels on the sheet (labels.Sheet: 2 mm). */
const GAP_MM = 2;

/**
 * Client-side preview of a label template (TEC-764; the backend has no
 * preview endpoint): one sheet row of sample labels laid out like the
 * printer's HTML (labels.Sheet) — size, columns, logo, QR or code128,
 * code text and name. The code128 bars are illustrative, not scannable.
 */
export function LabelPreview({ values }: { values: LabelTemplateValues }) {
  const { t } = useLocale();
  const width = clampNumber(values.width_mm, 20, 200, 70);
  const height = clampNumber(values.height_mm, 10, 200, 37);
  const columns = Math.round(clampNumber(values.columns, 1, 8, 3));
  const isQr = values.kind === "location" || values.symbology === "qr";
  const sample = PREVIEW_SAMPLE[values.kind];
  const logo =
    values.logo_mode === "text"
      ? values.logo_text.trim()
      : values.logo_mode === "image"
        ? values.logo_image
        : "";

  const sheetMm = columns * width + (columns - 1) * GAP_MM;
  const scale = Math.min(PX_PER_MM, PREVIEW_WIDTH_PX / sheetMm);
  const px = (mm: number) => `${(mm * scale).toFixed(1)}px`;
  const qrSide = qrSideMm({
    width,
    height,
    showName: values.show_name,
    showCodeText: values.show_code_text,
    logo: values.logo_mode !== "none",
  });

  const [qr, setQr] = useState<string | null>(null);
  useEffect(() => {
    if (!isQr) return;
    let alive = true;
    QRCode.toDataURL(qrPayload(values.kind, sample.code), {
      margin: 0,
      width: 160,
    })
      .then((src) => {
        if (alive) setQr(src);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [isQr, values.kind, sample.code]);

  return (
    <figure className="space-y-2" data-testid="label-preview">
      <div
        className="bg-muted/30 overflow-x-auto rounded-md border p-3"
        dir="ltr"
      >
        <div
          className="grid"
          style={{
            gridTemplateColumns: `repeat(${columns}, ${px(width)})`,
            gap: px(GAP_MM),
          }}
        >
          {Array.from({ length: columns }, (_, i) => (
            <div
              key={i}
              className="flex flex-col items-center justify-center overflow-hidden border border-dashed border-slate-300 bg-white text-center text-slate-900"
              style={{
                width: px(width),
                height: px(height),
                padding: px(1.5),
                gap: px(0.8),
              }}
              data-testid="label-preview-item"
              data-symbology={isQr ? "qr" : "code128"}
            >
              {values.logo_mode === "text" && logo ? (
                <div
                  className="font-bold"
                  style={{ fontSize: px(2.5), maxHeight: px(6) }}
                  data-testid="label-preview-logo"
                >
                  {logo}
                </div>
              ) : null}
              {values.logo_mode === "image" &&
              logo.startsWith("data:image/") ? (
                // eslint-disable-next-line @next/next/no-img-element -- data URI preview
                <img
                  src={logo}
                  alt=""
                  style={{ maxHeight: px(6), maxWidth: px(width - 4) }}
                  data-testid="label-preview-logo"
                />
              ) : null}
              {isQr ? (
                qr ? (
                  // eslint-disable-next-line @next/next/no-img-element -- data URI preview
                  <img
                    src={qr}
                    alt=""
                    style={{ width: px(qrSide), height: px(qrSide) }}
                  />
                ) : (
                  <div
                    className="bg-slate-200"
                    style={{ width: px(qrSide), height: px(qrSide) }}
                  />
                )
              ) : (
                <Bars code={sample.code} heightPx={Math.max(8, 10 * scale)} />
              )}
              {values.show_code_text ? (
                <div
                  className="font-mono break-all"
                  style={{ fontSize: px(2.6) }}
                >
                  {sample.code}
                </div>
              ) : null}
              {values.show_name ? (
                <>
                  <div
                    className="leading-tight font-bold"
                    style={{ fontSize: px(2.5) }}
                  >
                    {sample.title}
                  </div>
                  <div
                    className="leading-tight text-slate-600"
                    style={{ fontSize: px(2.1) }}
                  >
                    {sample.subtitle}
                  </div>
                </>
              ) : null}
            </div>
          ))}
        </div>
      </div>
      <figcaption className="text-muted-foreground text-xs">
        {t("stock.label_templates.preview.caption", {
          width: String(width),
          height: String(height),
          columns,
        })}
      </figcaption>
    </figure>
  );
}

function clampNumber(raw: string, min: number, max: number, fallback: number) {
  const n = Number(raw);
  if (!Number.isFinite(n) || raw.trim() === "") return fallback;
  return Math.min(max, Math.max(min, n));
}

/** Illustrative bars derived from the code (a preview, not code128). */
function Bars({ code, heightPx }: { code: string; heightPx: number }) {
  const widths: number[] = [2, 1, 1, 2];
  for (const ch of code) {
    const c = ch.charCodeAt(0);
    widths.push((c % 3) + 1, ((c >> 2) % 2) + 1, ((c >> 3) % 3) + 1);
  }
  widths.push(2, 3, 1, 1, 2);
  let x = 0;
  const rects = widths.map((w, i) => {
    const rect =
      i % 2 === 0 ? <rect key={i} x={x} y={0} width={w} height={10} /> : null;
    x += w;
    return rect;
  });
  return (
    <svg
      viewBox={`0 0 ${x} 10`}
      preserveAspectRatio="none"
      className="w-full fill-current"
      style={{ height: heightPx }}
      aria-hidden
    >
      {rects}
    </svg>
  );
}
