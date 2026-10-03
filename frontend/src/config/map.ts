/**
 * Map settings (TEC-242). Tiles come from OpenStreetMap: no API key, no
 * account; the attribution is required by the OSM tile usage policy.
 * Leaflet itself is bundled from npm (no CDN).
 */
export type LatLng = { lat: number; lng: number };

export const mapConfig = {
  tileUrl: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
  attribution:
    '&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer">OpenStreetMap</a>',
  maxZoom: 19,
  /** Centre of Türkiye: shown when the browser location is not available. */
  defaultCenter: { lat: 39.0, lng: 35.0 } satisfies LatLng,
  defaultZoom: 6,
  /** Zoom used once a single point (user or picked position) is known. */
  pointZoom: 12,
} as const;
