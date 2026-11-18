"use client";

import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
  XIcon,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Toaster as Sonner, type ToasterProps } from "sonner";

import { useLocale } from "@/providers/locale-provider";

/**
 * Uygulamanın tek toast alanı (TEC-1235). Görünüm `styles/toast.css`
 * içindeki marka tokenlarından gelir; açık/koyu tema `resolvedTheme` ile,
 * yön (RTL) dil ayarından. Konum mantıksal: LTR'de sağ üst, RTL'de sol üst.
 */
const Toaster = (props: ToasterProps) => {
  const { resolvedTheme } = useTheme();
  const { dir, t } = useLocale();
  const rtl = dir === "rtl";

  return (
    <Sonner
      theme={resolvedTheme === "dark" ? "dark" : "light"}
      dir={dir}
      position={rtl ? "top-left" : "top-right"}
      swipeDirections={rtl ? ["left", "top"] : ["right", "top"]}
      visibleToasts={4}
      closeButton
      containerAriaLabel={t("common.toast_region")}
      className="toaster app-toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" aria-hidden />,
        info: <InfoIcon className="size-4" aria-hidden />,
        warning: <TriangleAlertIcon className="size-4" aria-hidden />,
        error: <OctagonXIcon className="size-4" aria-hidden />,
        loading: <Loader2Icon className="size-4 animate-spin" aria-hidden />,
        close: <XIcon className="size-3" aria-hidden />,
      }}
      toastOptions={{
        closeButtonAriaLabel: t("common.close"),
        classNames: { toast: "app-toast" },
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      {...props}
    />
  );
};

export { Toaster };
