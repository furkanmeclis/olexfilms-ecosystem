/**
 * Vehicle catalog image rules (TEC-149 backend, TEC-150 UI). The backend
 * sniffs the bytes and refuses SVG with 400 (XSS risk, decision 3 on
 * TEC-101); the client checks first so the user gets a clear warning
 * instead of a generic upload error.
 */

export const VEHICLE_IMAGE_ACCEPT = "image/jpeg,image/png,image/webp";

const ALLOWED_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);
const ALLOWED_EXT = /\.(jpe?g|png|webp)$/i;

/** Upload limits (bytes), same as the Go storage limits. */
export const VEHICLE_IMAGE_LIMITS = {
  logo: 2 * 1024 * 1024,
  hero: 5 * 1024 * 1024,
} as const;

export type VehicleImageKind = keyof typeof VEHICLE_IMAGE_LIMITS;

export type VehicleImageProblem = "svg" | "type" | "size";

export function isSvgFile(file: Pick<File, "name" | "type">): boolean {
  return (
    file.type.toLowerCase().startsWith("image/svg") ||
    /\.svgz?$/i.test(file.name)
  );
}

/** Returns why a file cannot be uploaded, or null when it is fine. */
export function vehicleImageProblem(
  file: Pick<File, "name" | "type" | "size">,
  kind: VehicleImageKind,
): VehicleImageProblem | null {
  if (isSvgFile(file)) return "svg";
  const type = file.type.toLowerCase();
  if (type ? !ALLOWED_TYPES.has(type) : !ALLOWED_EXT.test(file.name)) {
    return "type";
  }
  if (file.size > VEHICLE_IMAGE_LIMITS[kind]) return "size";
  return null;
}

/** Fixed public logo URL of a brand (served by app/brand-logos/[uuid]). */
export function brandLogoUrl(uuid: string, version?: string | null): string {
  const base = `/brand-logos/${encodeURIComponent(uuid)}`;
  return version ? `${base}?v=${encodeURIComponent(version)}` : base;
}

/** `?v=` cache-buster of a backend logo_url, if any. */
export function logoVersion(logoUrl?: string | null): string | null {
  if (!logoUrl) return null;
  const q = logoUrl.indexOf("?");
  if (q < 0) return null;
  return new URLSearchParams(logoUrl.slice(q + 1)).get("v");
}
