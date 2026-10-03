"use client";

import QRCode from "qrcode";
import { useEffect, useState } from "react";

import { publicWarrantyPath } from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

/**
 * QR code of the public warranty page (/garanti/{code}, TEC-248), drawn in
 * the browser from the current origin; the code itself stays readable
 * under it.
 */
export function PortalWarrantyQr({ publicCode }: { publicCode: string }) {
  const { t } = useLocale();
  const [src, setSrc] = useState<string | null>(null);
  const path = publicWarrantyPath(publicCode);

  useEffect(() => {
    let alive = true;
    const url = `${window.location.origin}${path}`;
    QRCode.toDataURL(url, { margin: 1, width: 160 })
      .then((next) => {
        if (alive) setSrc(next);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [path]);

  return (
    <a
      href={path}
      target="_blank"
      rel="noreferrer"
      className="flex shrink-0 flex-col items-center gap-1"
      data-testid="warranty-qr"
      data-code={publicCode}
    >
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element -- data: URL
        <img
          src={src}
          alt={t("portal.vehicle.qr_alt")}
          width={96}
          height={96}
          className="rounded-md border bg-white p-1"
        />
      ) : (
        <span className="bg-muted size-24 rounded-md" aria-hidden="true" />
      )}
      <span className="font-mono text-[10px] tracking-wider" dir="ltr">
        {publicCode}
      </span>
    </a>
  );
}
