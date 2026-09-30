import { ImageResponse } from "next/og";

import {
  markAccentPath,
  markGlyphPath,
  markViewBox,
} from "@/components/brand/artwork";
import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/** Apple touch / PWA icon — OP lockup on espresso. */
export default function AppleIcon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: brand.colors.ink,
        borderRadius: 40,
      }}
    >
      <svg width="148" height="148" viewBox={markViewBox}>
        <path
          fill={brand.colors.primary}
          fillRule="evenodd"
          d={markAccentPath}
        />
        <path fill={brand.colors.white} d={markGlyphPath} />
      </svg>
    </div>,
    { ...size },
  );
}
