"use client";

import {
  CalendarPlus,
  LocateFixed,
  MapPin,
  MessageCircle,
  Star,
  Store,
} from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";

import { LeafletMap, type MapMarker } from "@/components/common/leaflet-map";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { mapConfig, type LatLng } from "@/config/map";
import { routes } from "@/config/routes";
import {
  FALLBACK_RADIUS_KM,
  LOCATED_RADIUS_KM,
  RADIUS_OPTIONS,
  fetchNearbyDealers,
  formatDistanceKm,
  locateUser,
  nearbyQueryString,
  whatsappHref,
  type NearbyDealer,
  type NearbyResult,
  type PositionResult,
  type RadiusKm,
} from "@/features/dealers/lib/dealers";
import { PortalPage } from "@/features/portal/components/portal-page";
import { usePortalReadOnly } from "@/features/portal/lib/use-portal-read-only";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Notice = "denied" | "unavailable" | "unsupported";

const NOTICE_KEYS: Record<Notice, string> = {
  denied: "portal.dealers.notice_denied",
  unavailable: "portal.dealers.notice_unavailable",
  unsupported: "portal.dealers.notice_unsupported",
};

const USER_MARKER_ID = "__search_point";
const NO_DEALERS: NearbyDealer[] = [];

function browserGeolocation() {
  return typeof navigator !== "undefined" && "geolocation" in navigator
    ? navigator.geolocation
    : null;
}

/**
 * "Find a dealer" (TEC-242), public: no portal session needed, linked from
 * the landing page. Asks for the browser position; when it is refused or
 * missing the map opens on the default centre with a notice, and a tap on
 * the map searches around that point instead.
 */
export function DealerFinder() {
  const { t, locale } = useLocale();
  const [locating, setLocating] = useState(true);
  const [center, setCenter] = useState<LatLng>({ ...mapConfig.defaultCenter });
  const [origin, setOrigin] = useState<"user" | "default" | "picked">(
    "default",
  );
  const [notice, setNotice] = useState<Notice | null>(null);
  const [radius, setRadius] = useState<RadiusKm>(LOCATED_RADIUS_KM);
  const [result, setResult] = useState<{
    key: string;
    value: NearbyResult;
  } | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  // TEC-327: a fleet session cannot book, so no "book" shortcut.
  const canBook = !usePortalReadOnly();

  const applyPosition = useCallback((pos: PositionResult) => {
    setCenter(pos.center);
    setSelected(null);
    if (pos.kind === "located") {
      setOrigin("user");
      setNotice(null);
      setRadius(LOCATED_RADIUS_KM);
    } else {
      setOrigin("default");
      setNotice(pos.reason);
      setRadius(FALLBACK_RADIUS_KM);
    }
    setLocating(false);
  }, []);

  useEffect(() => {
    let active = true;
    void locateUser(browserGeolocation()).then((pos) => {
      if (active) applyPosition(pos);
    });
    return () => {
      active = false;
    };
  }, [applyPosition]);

  const relocate = () => {
    setLocating(true);
    void locateUser(browserGeolocation()).then(applyPosition);
  };

  const query = useMemo(
    () => ({ lat: center.lat, lng: center.lng, radiusKm: radius }),
    [center, radius],
  );
  const queryKey = `${nearbyQueryString(query)}#${retry}`;

  useEffect(() => {
    if (locating) return;
    const controller = new AbortController();
    void fetchNearbyDealers(query, fetch, controller.signal).then((value) => {
      if (!controller.signal.aborted) setResult({ key: queryKey, value });
    });
    return () => controller.abort();
  }, [locating, query, queryKey]);

  const loading = locating || result?.key !== queryKey;
  const dealers =
    !loading && result?.value.kind === "ok" ? result.value.dealers : NO_DEALERS;

  const markers = useMemo<MapMarker[]>(() => {
    const list: MapMarker[] = [
      {
        id: USER_MARKER_ID,
        lat: center.lat,
        lng: center.lng,
        kind: "point",
        label:
          origin === "user"
            ? t("portal.dealers.your_location")
            : t("portal.dealers.search_point"),
      },
    ];
    for (const d of dealers) {
      list.push({
        id: d.slug,
        lat: d.latitude,
        lng: d.longitude,
        label: d.name,
        active: d.slug === selected,
      });
    }
    return list;
  }, [center, origin, dealers, selected, t]);

  const focus = useMemo(() => {
    const d = dealers.find((x) => x.slug === selected);
    return d ? { lat: d.latitude, lng: d.longitude } : null;
  }, [dealers, selected]);

  const zoom = origin === "default" ? mapConfig.defaultZoom : 9;

  return (
    <PortalPage
      title={t("portal.dealers.title")}
      icon={<Store className="size-5" aria-hidden />}
      back={{ href: "/", label: t("portal.dealers.back_home") }}
      testId="dealer-finder"
    >
      <p className="text-muted-foreground text-sm">
        {t("portal.dealers.subtitle")}
      </p>

      {notice ? (
        <Alert data-testid="dealer-location-notice" data-reason={notice}>
          <MapPin className="size-4" aria-hidden />
          <AlertDescription>{t(NOTICE_KEYS[notice])}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-40 flex-col gap-1">
          <span id="dealer-radius-label" className="text-sm font-medium">
            {t("portal.dealers.radius_label")}
          </span>
          <Select
            value={String(radius)}
            onValueChange={(v) => setRadius(Number(v) as RadiusKm)}
          >
            <SelectTrigger aria-labelledby="dealer-radius-label">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RADIUS_OPTIONS.map((km) => (
                <SelectItem key={km} value={String(km)}>
                  {t("portal.dealers.radius_option", { km })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={relocate}
          disabled={locating}
          data-testid="dealer-use-location"
        >
          <LocateFixed className="size-4" aria-hidden />
          {locating
            ? t("portal.dealers.locating")
            : t("portal.dealers.use_my_location")}
        </Button>
      </div>

      <div className="space-y-1">
        <LeafletMap
          center={center}
          zoom={zoom}
          markers={markers}
          fitToMarkers={!selected}
          focus={focus}
          ariaLabel={t("portal.dealers.map_label")}
          testId="dealer-map"
          className="h-80"
          onMapClick={(point) => {
            setCenter(point);
            setOrigin("picked");
            setSelected(null);
          }}
          onMarkerClick={(id) => {
            if (id !== USER_MARKER_ID) setSelected(id);
          }}
        />
        <p className="text-muted-foreground text-xs">
          {t("portal.dealers.pick_hint")}
        </p>
      </div>

      <DealerResults
        loading={loading}
        result={loading ? null : (result?.value ?? null)}
        locale={locale}
        selected={selected}
        onSelect={setSelected}
        onRetry={() => setRetry((n) => n + 1)}
        canBook={canBook}
      />
    </PortalPage>
  );
}

function DealerResults({
  loading,
  result,
  locale,
  selected,
  onSelect,
  onRetry,
  canBook,
}: {
  loading: boolean;
  result: NearbyResult | null;
  locale: string;
  selected: string | null;
  onSelect: (slug: string) => void;
  onRetry: () => void;
  canBook: boolean;
}) {
  const { t } = useLocale();
  if (loading || !result) {
    return (
      <p
        className="text-muted-foreground text-sm"
        data-testid="dealer-loading"
        aria-live="polite"
      >
        {t("portal.dealers.loading")}
      </p>
    );
  }
  if (result.kind !== "ok") {
    return (
      <Alert variant="destructive" data-testid="dealer-error">
        <AlertDescription className="flex flex-wrap items-center gap-3">
          {result.kind === "rate_limited"
            ? t("portal.dealers.rate_limited")
            : t("portal.dealers.error")}
          <Button type="button" size="sm" variant="outline" onClick={onRetry}>
            {t("portal.dealers.retry")}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }
  if (result.dealers.length === 0) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="dealer-empty">
        {t("portal.dealers.empty")}
      </p>
    );
  }
  return (
    <section className="space-y-3" aria-live="polite">
      <h2 className="text-sm font-medium">
        {t("portal.dealers.count", { count: result.dealers.length })}
      </h2>
      <ul className="space-y-3" data-testid="dealer-list">
        {result.dealers.map((d) => {
          const wa = whatsappHref(d.whatsapp);
          const place = [d.district, d.city].filter(Boolean).join(", ");
          return (
            <li key={d.slug}>
              <Card
                data-testid="dealer-item"
                data-slug={d.slug}
                className={cn(
                  "transition-colors",
                  d.slug === selected && "border-primary",
                )}
              >
                <CardContent className="flex flex-wrap items-center justify-between gap-3 py-4">
                  <div className="min-w-0 space-y-1">
                    <p className="truncate font-medium">{d.name}</p>
                    <p className="text-muted-foreground text-sm">
                      {place ? `${place} · ` : ""}
                      <span data-testid="dealer-distance">
                        {formatDistanceKm(d.distance_km, locale)}
                      </span>
                    </p>
                    {typeof d.google_rating === "number" ? (
                      <p
                        className="text-muted-foreground flex items-center gap-1 text-xs"
                        data-testid="dealer-rating"
                      >
                        <Star
                          className="size-3 fill-amber-500 text-amber-500"
                          aria-hidden
                        />
                        {t("portal.dealers.google_rating", {
                          rating: d.google_rating.toFixed(1),
                        })}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {d.has_showcase ? (
                      <Button asChild size="sm" variant="outline">
                        <Link
                          href={routes.public.dealer(d.slug)}
                          data-testid="dealer-showcase"
                        >
                          <Store className="size-4" aria-hidden />
                          {t("portal.dealers.showcase")}
                        </Link>
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => onSelect(d.slug)}
                    >
                      <MapPin className="size-4" aria-hidden />
                      {t("portal.dealers.show_on_map")}
                    </Button>
                    {canBook && d.accepts_appointments && d.uuid ? (
                      <Button asChild size="sm" variant="outline">
                        <Link
                          href={routes.portal.newAppointment({
                            dealer: { uuid: d.uuid, name: d.name },
                          })}
                          data-testid="dealer-book"
                        >
                          <CalendarPlus className="size-4" aria-hidden />
                          {t("portal.appointments.book")}
                        </Link>
                      </Button>
                    ) : null}
                    {wa ? (
                      <Button asChild size="sm">
                        <a
                          href={wa}
                          target="_blank"
                          rel="noopener noreferrer"
                          data-testid="dealer-whatsapp"
                        >
                          <MessageCircle className="size-4" aria-hidden />
                          {t("portal.dealers.whatsapp")}
                        </a>
                      </Button>
                    ) : null}
                  </div>
                </CardContent>
              </Card>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
