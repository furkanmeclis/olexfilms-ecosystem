import { isApiError } from "@/lib/api";
import type {
  IntakePhotoList,
  PhotoAngle,
} from "@/features/photo-standard/services/photo-standard.service";

/** Server upload limit (TEC-498): 12 MB, checked before sending. */
export const MAX_PHOTO_BYTES = 12 * 1024 * 1024;

/** 422 code of a service whose required angles are not all photographed. */
export const PHOTO_STANDARD_INCOMPLETE = "PHOTO_STANDARD_INCOMPLETE";

export type PhotoFileError = "too_large" | "not_image";

/**
 * Client check of a picked photo: an image of at most 12 MB. The original
 * is sent as is (no client-side resize); the server sniffs the bytes.
 */
export function validatePhotoFile(file: File): PhotoFileError | null {
  if (file.size > MAX_PHOTO_BYTES) return "too_large";
  // iOS may hand HEIC files over without a MIME type.
  if (file.type && !file.type.startsWith("image/")) return "not_image";
  return null;
}

/** Text of a localized map: UI language, else Turkish, English, any. */
export function localizedText(
  map: Record<string, string> | null | undefined,
  locale: string,
): string {
  if (!map) return "";
  return (
    map[locale]?.trim() ||
    map.tr?.trim() ||
    map.en?.trim() ||
    Object.values(map).find((v) => v?.trim()) ||
    ""
  );
}

export function angleName(angle: PhotoAngle, locale: string): string {
  return localizedText(angle.name, locale) || angle.key;
}

/** Keys of the required angles still without a photo. */
export function missingAngles(intake: IntakePhotoList | undefined): string[] {
  return intake?.missing ?? [];
}

/**
 * Missing angle keys of a 422 PHOTO_STANDARD_INCOMPLETE answer (one
 * `intake_photos.<key>` detail per angle, also data.missing_angles); null
 * for any other error.
 */
export function missingAnglesFromError(error: unknown): string[] | null {
  if (!isApiError(error) || error.code !== PHOTO_STANDARD_INCOMPLETE) {
    return null;
  }
  const fromDetails = error.details
    .map((d) => d.field ?? "")
    .filter((f) => f.startsWith("intake_photos."))
    .map((f) => f.slice("intake_photos.".length));
  if (fromDetails.length > 0) return fromDetails;
  const data = (error.body as { data?: { missing_angles?: unknown } } | null)
    ?.data;
  return Array.isArray(data?.missing_angles)
    ? data.missing_angles.filter((k): k is string => typeof k === "string")
    : [];
}

/**
 * KVKK (TEC-499): the EXIF location is for super admins, the center and an
 * owner of the service's own dealer. The API already redacts it; the UI
 * applies the same rule so a stray value is never shown to staff.
 */
export function canSeeIntakeLocation(input: {
  isSuperAdmin: boolean;
  orgType: string | undefined;
  orgUuid: string | undefined;
  orgRoles: readonly string[];
  serviceOrgUuid: string;
}): boolean {
  if (input.isSuperAdmin || input.orgType === "center") return true;
  return (
    input.orgUuid === input.serviceOrgUuid &&
    input.orgRoles.includes("dealer_owner")
  );
}

/** Map link of an EXIF position, null without a valid one. */
export function mapLink(
  lat: string | undefined,
  lng: string | undefined,
): string | null {
  const la = Number(lat);
  const lo = Number(lng);
  if (!lat || !lng || !Number.isFinite(la) || !Number.isFinite(lo)) {
    return null;
  }
  return `https://www.openstreetmap.org/?mlat=${la}&mlon=${lo}#map=17/${la}/${lo}`;
}

/** Gap between two consecutive sort orders after a drag-and-drop. */
export const SORT_STEP = 10;

/** sort_order of every dragged angle whose position changed. */
export function reorderPatches(
  rows: readonly PhotoAngle[],
): { angle: PhotoAngle; sort_order: number }[] {
  const out: { angle: PhotoAngle; sort_order: number }[] = [];
  rows.forEach((angle, index) => {
    const sortOrder = (index + 1) * SORT_STEP;
    if (angle.sort_order !== sortOrder)
      out.push({ angle, sort_order: sortOrder });
  });
  return out;
}

export function nextSortOrder(rows: readonly PhotoAngle[]): number {
  const max = rows.reduce((acc, row) => Math.max(acc, row.sort_order), 0);
  return Math.ceil((max + 1) / SORT_STEP) * SORT_STEP;
}

/** Full PUT body of an angle with some fields changed. */
export function angleInput(
  angle: PhotoAngle,
  patch: Partial<Pick<PhotoAngle, "required" | "active" | "sort_order">>,
) {
  return {
    key: angle.key,
    name: angle.name,
    hint: angle.hint,
    example_storage_key: angle.example_storage_key ?? null,
    required: patch.required ?? angle.required,
    active: patch.active ?? angle.active,
    sort_order: patch.sort_order ?? angle.sort_order,
  };
}

/** One row of the override form. */
export type OverrideRow = {
  key: string;
  overridden: boolean;
  required: boolean;
  hidden: boolean;
  defaultRequired: boolean;
  defaultHidden: boolean;
};

export function overrideRows(angles: readonly PhotoAngle[]): OverrideRow[] {
  return angles.map((a) => {
    const defaultRequired = a.default_required ?? a.required;
    const defaultHidden = a.default_hidden ?? false;
    const overridden = Boolean(a.overridden);
    return {
      key: a.key,
      overridden,
      required: overridden ? a.required : defaultRequired,
      hidden: overridden ? Boolean(a.hidden) : defaultHidden,
      defaultRequired,
      defaultHidden,
    };
  });
}

/** Sets a switch: the row becomes an override; hidden clears required. */
export function setOverride(
  row: OverrideRow,
  field: "required" | "hidden",
  value: boolean,
): OverrideRow {
  const next = { ...row, overridden: true, [field]: value };
  if (next.hidden) next.required = false;
  return next;
}

export function resetOverride(row: OverrideRow): OverrideRow {
  return {
    ...row,
    overridden: false,
    required: row.defaultRequired,
    hidden: row.defaultHidden,
  };
}

/** PUT body: only the overridden rows (the rest follow the center). */
export function overridesBody(organizationUuid: string, rows: OverrideRow[]) {
  return {
    organization_uuid: organizationUuid,
    overrides: rows
      .filter((r) => r.overridden)
      .map((r) => ({
        angle_key: r.key,
        required: r.hidden ? false : r.required,
        hidden: r.hidden,
      })),
  };
}
