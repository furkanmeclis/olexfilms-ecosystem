"use client";

import { useQuery } from "@tanstack/react-query";
import { QrCode } from "lucide-react";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  extractServiceNo,
  isValidServiceNo,
} from "@/features/services/lib/service-no";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

type Detector = {
  detect: (source: CanvasImageSource) => Promise<{ rawValue: string }[]>;
};
type DetectorCtor = new (opts: { formats: string[] }) => Detector;

function barcodeDetector(): DetectorCtor | null {
  if (typeof window === "undefined") return null;
  const ctor = (window as unknown as { BarcodeDetector?: DetectorCtor })
    .BarcodeDetector;
  return ctor ?? null;
}

const noopSubscribe = () => () => {};

/**
 * Camera QR reader of the warranty card (the browser's BarcodeDetector:
 * Chrome / Edge / Android; elsewhere the field takes a pasted URL or a
 * USB / Bluetooth scanner, which types like a keyboard).
 */
function QrScanDialog({
  open,
  onClose,
  onScan,
}: {
  open: boolean;
  onClose: () => void;
  onScan: (text: string) => void;
}) {
  const { t } = useLocale();
  const video = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const Ctor = barcodeDetector();
    if (!Ctor) return;
    let stream: MediaStream | null = null;
    let frame = 0;
    let stopped = false;
    const detector = new Ctor({ formats: ["qr_code"] });
    const loop = async () => {
      if (stopped || !video.current) return;
      try {
        const codes = await detector.detect(video.current);
        if (codes.length > 0 && !stopped) {
          onScan(codes[0].rawValue);
          return;
        }
      } catch {
        // A frame that is not ready yet: try the next one.
      }
      frame = requestAnimationFrame(() => void loop());
    };
    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: "environment" } })
      .then(async (s) => {
        stream = s;
        if (stopped || !video.current) return;
        video.current.srcObject = s;
        await video.current.play();
        void loop();
      })
      .catch(() => setError(t("services.service_no.camera_denied")));
    return () => {
      stopped = true;
      cancelAnimationFrame(frame);
      stream?.getTracks().forEach((track) => track.stop());
    };
  }, [open, onScan, t]);

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("services.service_no.scan_title")}</DialogTitle>
          <DialogDescription>
            {t("services.service_no.scan_description")}
          </DialogDescription>
        </DialogHeader>
        {error ? (
          <p className="text-destructive text-sm">{error}</p>
        ) : (
          <video
            ref={video}
            className="aspect-square w-full rounded-md bg-black object-cover"
            muted
            playsInline
            data-testid="service-no-video"
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

/**
 * Service number field (TEC-1004, the legacy ServiceNumberInput): typed,
 * pasted (the card URL is reduced to its number) or scanned from the
 * warranty card's QR; the number is checked as it changes and a number
 * used by another service is flagged. Empty on a new service means a
 * generated DS number.
 */
export function ServiceNoField({
  value,
  onChange,
  serviceUuid,
  disabled,
  optional,
}: {
  value: string;
  onChange: (value: string) => void;
  /** The service being edited: its own number counts as available. */
  serviceUuid?: string;
  disabled?: boolean;
  /** A new service: blank allocates a number. */
  optional?: boolean;
}) {
  const { t } = useLocale();
  const [scanning, setScanning] = useState(false);
  // Client-only capability: false on the server, real after hydration.
  const canScan = useSyncExternalStore(
    noopSubscribe,
    () => barcodeDetector() !== null,
    () => false,
  );

  const no = useDebounce(value.trim(), 400);
  const valid = isValidServiceNo(no);
  const check = useQuery({
    queryKey: serviceWizardKeys.serviceNo(no, serviceUuid),
    queryFn: () => serviceWizardService.serviceNoAvailability(no, serviceUuid),
    enabled: !disabled && valid,
    staleTime: 10_000,
  });

  let hint: { text: string; tone: string } | null = null;
  if (disabled) {
    hint = { text: t("services.service_no.locked"), tone: "muted" };
  } else if (!no) {
    hint = optional
      ? { text: t("services.service_no.auto"), tone: "muted" }
      : null;
  } else if (!valid) {
    hint = { text: t("services.service_no.invalid"), tone: "destructive" };
  } else if (check.data && !check.data.available) {
    hint = { text: t("services.service_no.taken"), tone: "destructive" };
  } else if (check.data?.available) {
    hint = { text: t("services.service_no.available"), tone: "success" };
  }

  const set = (text: string) => onChange(extractServiceNo(text));

  return (
    <div className="max-w-sm space-y-2" data-testid="service-no-field">
      <Label htmlFor="service-no">{t("services.service_no.label")}</Label>
      <div className="flex gap-2">
        <Input
          id="service-no"
          name="service_no"
          dir="ltr"
          className="font-mono"
          autoComplete="off"
          spellCheck={false}
          maxLength={256}
          value={value}
          disabled={disabled}
          placeholder={optional ? t("services.service_no.placeholder") : ""}
          aria-invalid={hint?.tone === "destructive" ? true : undefined}
          onChange={(e) => onChange(e.target.value)}
          onBlur={(e) => set(e.target.value)}
          onPaste={(e) => {
            e.preventDefault();
            set(e.clipboardData.getData("text"));
          }}
          onKeyDown={(e) => {
            // A hand-held scanner ends the card URL with Enter.
            if (e.key === "Enter") {
              e.preventDefault();
              set(e.currentTarget.value);
            }
          }}
        />
        {canScan && !disabled ? (
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("services.service_no.scan")}
            data-testid="service-no-scan"
            onClick={() => setScanning(true)}
          >
            <QrCode />
          </Button>
        ) : null}
      </div>
      {hint ? (
        <p
          className={
            hint.tone === "destructive"
              ? "text-destructive text-xs"
              : hint.tone === "success"
                ? "text-xs text-emerald-600 dark:text-emerald-400"
                : "text-muted-foreground text-xs"
          }
          data-testid="service-no-hint"
        >
          {hint.text}
        </p>
      ) : null}
      <QrScanDialog
        open={scanning}
        onClose={() => setScanning(false)}
        onScan={(text) => {
          setScanning(false);
          set(text);
        }}
      />
    </div>
  );
}
