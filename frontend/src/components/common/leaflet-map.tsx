"use client";

import "leaflet/dist/leaflet.css";

import type * as Leaflet from "leaflet";
import { useEffect, useRef, useState } from "react";

import { mapConfig, type LatLng } from "@/config/map";
import { cn } from "@/lib/utils";

export type MapMarker = {
  id: string;
  lat: number;
  lng: number;
  /** Tooltip text (dealer name, "your location"…). */
  label?: string;
  /** "point" is the user / picked position, "place" a dealer. */
  kind?: "point" | "place";
  active?: boolean;
  /** TEC-496: data markers (region circles, clusters) size and color. */
  radius?: number;
  color?: string;
};

export type LeafletMapProps = {
  center: LatLng;
  zoom?: number;
  markers?: MapMarker[];
  /** Zooms out so every marker is visible whenever the marker set changes. */
  fitToMarkers?: boolean;
  /** Pans here (keeping the zoom) when it changes, e.g. a selected dealer. */
  focus?: LatLng | null;
  onMapClick?: (point: LatLng) => void;
  onMarkerClick?: (id: string) => void;
  ariaLabel: string;
  className?: string;
  testId?: string;
};

const MARKER_STYLE: Record<
  "point" | "place" | "active",
  Leaflet.CircleMarkerOptions
> = {
  point: {
    radius: 8,
    color: "#ffffff",
    weight: 3,
    fillColor: "#2563eb",
    fillOpacity: 1,
  },
  place: {
    radius: 9,
    color: "#ffffff",
    weight: 2,
    fillColor: "#dc2626",
    fillOpacity: 0.9,
  },
  active: {
    radius: 12,
    color: "#ffffff",
    weight: 3,
    fillColor: "#b91c1c",
    fillOpacity: 1,
  },
};

/**
 * Reusable OpenStreetMap map (TEC-242): the dealer finder, the panel
 * coordinate picker and the public dealer page (F2-04c) use it. Leaflet is
 * loaded on the client only (it touches `window`); markers are circle
 * markers, so no icon images have to be served. The map box stays
 * left-to-right (tiles and controls), the page around it follows the locale.
 */
export function LeafletMap({
  center,
  zoom = mapConfig.defaultZoom,
  markers = [],
  fitToMarkers = false,
  focus,
  onMapClick,
  onMarkerClick,
  ariaLabel,
  className,
  testId,
}: LeafletMapProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<Leaflet.Map | null>(null);
  const layerRef = useRef<Leaflet.LayerGroup | null>(null);
  const libRef = useRef<typeof Leaflet | null>(null);
  const clickRef = useRef(onMapClick);
  const markerClickRef = useRef(onMarkerClick);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    clickRef.current = onMapClick;
    markerClickRef.current = onMarkerClick;
  }, [onMapClick, onMarkerClick]);

  // Create the map once; the initial view comes from the first props.
  const initial = useRef({ center, zoom });
  useEffect(() => {
    let cancelled = false;
    void import("leaflet").then((mod) => {
      const L = (mod as { default?: typeof Leaflet }).default ?? mod;
      if (cancelled || !hostRef.current || mapRef.current) return;
      libRef.current = L;
      const map = L.map(hostRef.current, {
        center: [initial.current.center.lat, initial.current.center.lng],
        zoom: initial.current.zoom,
        scrollWheelZoom: false,
      });
      L.tileLayer(mapConfig.tileUrl, {
        attribution: mapConfig.attribution,
        maxZoom: mapConfig.maxZoom,
      }).addTo(map);
      map.on("click", (event: Leaflet.LeafletMouseEvent) => {
        clickRef.current?.({ lat: event.latlng.lat, lng: event.latlng.lng });
      });
      layerRef.current = L.layerGroup().addTo(map);
      mapRef.current = map;
      setReady(true);
    });
    return () => {
      cancelled = true;
      mapRef.current?.remove();
      mapRef.current = null;
      layerRef.current = null;
      setReady(false);
    };
  }, []);

  useEffect(() => {
    if (!ready) return;
    mapRef.current?.setView([center.lat, center.lng], zoom);
  }, [ready, center.lat, center.lng, zoom]);

  useEffect(() => {
    const L = libRef.current;
    const map = mapRef.current;
    const layer = layerRef.current;
    if (!ready || !L || !map || !layer) return;
    layer.clearLayers();
    for (const m of markers) {
      const style = m.active
        ? MARKER_STYLE.active
        : MARKER_STYLE[m.kind ?? "place"];
      const marker = L.circleMarker([m.lat, m.lng], {
        ...style,
        ...(m.radius ? { radius: m.radius } : {}),
        ...(m.color ? { fillColor: m.color, fillOpacity: 0.7 } : {}),
      });
      if (m.label) marker.bindTooltip(m.label, { direction: "top" });
      marker.on("click", () => markerClickRef.current?.(m.id));
      marker.addTo(layer);
    }
    if (fitToMarkers && markers.length > 1) {
      map.fitBounds(L.latLngBounds(markers.map((m) => [m.lat, m.lng])), {
        padding: [32, 32],
        maxZoom: mapConfig.pointZoom,
      });
    }
  }, [ready, markers, fitToMarkers]);

  useEffect(() => {
    if (!ready || !focus) return;
    mapRef.current?.panTo([focus.lat, focus.lng]);
  }, [ready, focus]);

  return (
    <div
      ref={hostRef}
      dir="ltr"
      role="region"
      aria-label={ariaLabel}
      data-testid={testId}
      className={cn(
        "bg-muted isolate h-72 w-full overflow-hidden rounded-lg border",
        className,
      )}
    />
  );
}
