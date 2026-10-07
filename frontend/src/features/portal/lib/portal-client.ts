/**
 * Browser client of the customer / fleet portal (TEC-90). It talks only to
 * the portal BFF (/api/portal/v1) and the portal Auth.js instance
 * (/api/portal-auth), never to the panel endpoints, so the panel session
 * cookie is never read or written from the portal.
 */
import type { components } from "@/generated/api";

export const PORTAL_API_BASE = "/api/portal/v1";
export const PORTAL_AUTH_BASE = "/api/portal-auth";

export class PortalApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string | null,
    readonly details: { field?: string; message?: string; code?: string }[],
  ) {
    super(message);
    this.name = "PortalApiError";
  }
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?:
    | {
        code?: string;
        message?: string;
        details?: { field?: string; message?: string; code?: string }[];
      }
    | string;
};

async function parse<T>(response: Response): Promise<T> {
  let body: Envelope<T> | null = null;
  try {
    body = (await response.json()) as Envelope<T>;
  } catch {
    body = null;
  }
  if (!response.ok) {
    const err = body?.error;
    const code = typeof err === "string" ? err : (err?.code ?? null);
    const message =
      typeof err === "object" && err?.message
        ? err.message
        : `Request failed (${response.status})`;
    const details = typeof err === "object" ? (err?.details ?? []) : [];
    throw new PortalApiError(message, response.status, code, details);
  }
  return (body?.data ?? (undefined as T)) as T;
}

let refreshing: Promise<boolean> | null = null;

/** One refresh at a time; the BFF rotates the portal cookie. */
function refreshPortalSession(): Promise<boolean> {
  refreshing ??= fetch(`${PORTAL_API_BASE}/auth/refresh`, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: "{}",
  })
    .then((r) => r.ok)
    .catch(() => false)
    .finally(() => {
      refreshing = null;
    });
  return refreshing;
}

export async function portalRequest<T>(
  path: string,
  init: { method?: string; body?: unknown; locale?: string } = {},
): Promise<T> {
  const send = () =>
    fetch(`${PORTAL_API_BASE}/${path.replace(/^\//, "")}`, {
      method: init.method ?? "GET",
      credentials: "include",
      cache: "no-store",
      headers: {
        Accept: "application/json",
        ...(init.body !== undefined
          ? { "Content-Type": "application/json" }
          : {}),
        ...(init.locale ? { "Accept-Language": init.locale } : {}),
      },
      body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
    });
  let response = await send();
  if (response.status === 401 && !path.startsWith("auth/")) {
    if (await refreshPortalSession()) response = await send();
  }
  return parse<T>(response);
}

/**
 * Credentials sign-in against the portal Auth.js instance (the same calls
 * as the Auth.js client `signIn`, pinned to /api/portal-auth). Returns the
 * error code (`CredentialsSignin` subtype code) or null on success.
 */
export async function portalSignIn(
  provider: "phone-otp" | "fleet-password",
  fields: Record<string, string>,
): Promise<string | null> {
  const csrf = await fetch(`${PORTAL_AUTH_BASE}/csrf`, {
    credentials: "include",
    cache: "no-store",
  });
  if (!csrf.ok) return "unavailable";
  const { csrfToken } = (await csrf.json()) as { csrfToken?: string };
  if (!csrfToken) return "unavailable";

  const response = await fetch(`${PORTAL_AUTH_BASE}/callback/${provider}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-Auth-Return-Redirect": "1",
    },
    body: new URLSearchParams({
      ...fields,
      csrfToken,
      callbackUrl: window.location.href,
    }),
  });
  let url: string | null = null;
  try {
    url = ((await response.json()) as { url?: string }).url ?? null;
  } catch {
    url = null;
  }
  return signInErrorCode(response.status, url);
}

/** Error code from an Auth.js credentials callback answer; null = signed in. */
export function signInErrorCode(
  status: number,
  url: string | null,
): string | null {
  if (status >= 500) return "unavailable";
  if (!url) return status < 400 ? null : "CredentialsSignin";
  const parsed = new URL(url, "http://localhost");
  const error = parsed.searchParams.get("error");
  if (!error) return null;
  return parsed.searchParams.get("code") ?? error;
}

/** Signs out of the portal only (revokes the refresh token, clears the portal cookie). */
export async function portalSignOut(): Promise<void> {
  await fetch(`${PORTAL_API_BASE}/auth/logout`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  }).catch(() => undefined);
}

export type OTPRequestResult = {
  status: string;
  channel: string;
  expires_at: string;
  resend_at: string;
};

export type PendingLegalText = {
  uuid: string;
  kind: string;
  locale: string;
  version: number;
  body: string;
};

export type PortalWarranty = components["schemas"]["Warranty"];
export type PortalWarrantyClaimStatus =
  components["schemas"]["PortalWarrantyClaimStatus"];

export type PortalWarrantyPage = {
  items: PortalWarranty[];
  total: number;
  limit: number;
  offset: number;
};

/** TEC-238 / TEC-239 portal projections (no measurement data, no prices). */
export type PortalVehicle = components["schemas"]["PortalVehicle"];
export type PortalVehicleDetail = components["schemas"]["PortalVehicleDetail"];
export type PortalActiveWarranty =
  components["schemas"]["PortalActiveWarranty"];
export type PortalServiceListItem =
  components["schemas"]["PortalServiceListItem"];
export type PortalService = components["schemas"]["PortalService"];
export type PortalServiceProduct =
  components["schemas"]["PortalServiceProduct"];
export type PortalServiceWarranty =
  components["schemas"]["PortalServiceWarranty"];

/** TEC-243: the TEC-190 transfer, as the portal owner sees it. */
export type PortalVehicleTransfer = components["schemas"]["VehicleTransfer"];
export type PortalVehicleTransferVerifyInput =
  components["schemas"]["VehicleTransferVerifyInput"];

/** TEC-245 / TEC-288: an executed vehicle intake contract of the user. */
export type PortalContract = components["schemas"]["PortalContract"];
/**
 * Executed contract PDF through the portal BFF (TEC-288); the bytes come
 * straight back (no export job), so a plain download link is enough.
 */
export function portalContractPdfUrl(contractUuid: string): string {
  return `${PORTAL_API_BASE}/portal/contracts/${encodeURIComponent(contractUuid)}/pdf`;
}

/** TEC-245: the F0-10 notification preferences, as the portal reads them. */
export type PortalNotificationPreferences =
  components["schemas"]["NotificationPreferences"];

/** TEC-327: portal appointments (F3-04c) and a dealer's bookable days. */
export type PortalAppointment = components["schemas"]["PortalAppointment"];
export type PortalAppointmentInput =
  components["schemas"]["PortalAppointmentInput"];
export type AppointmentAvailabilityDay =
  components["schemas"]["AppointmentAvailabilityDay"];
export type AppointmentSlot = components["schemas"]["AppointmentSlot"];
export type PortalAppointmentPeriod = "upcoming" | "past";

export type PortalPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export const portalApi = {
  requestOTP(phone: string, country: string, locale: string) {
    return portalRequest<OTPRequestResult>("auth/otp/request", {
      method: "POST",
      body: { phone, country, locale, purpose: "customer_login" },
    });
  },
  me() {
    return portalRequest<{
      user: { name?: string; surname?: string; email?: string | null };
      /** Global roles: customer and / or fleet (TEC-245 read-only check). */
      roles?: string[];
    }>("auth/me");
  },
  pendingConsents(locale: string) {
    return portalRequest<{ items: PendingLegalText[] }>(
      `portal/consents/pending?locale=${encodeURIComponent(locale)}`,
    );
  },
  decideConsent(text: PendingLegalText, accepted: boolean) {
    return portalRequest("portal/consents", {
      method: "POST",
      body: {
        kind: text.kind,
        locale: text.locale,
        version: text.version,
        accepted,
      },
    });
  },
  /** Claim statuses of the user's warranties (TEC-339; no description / photos). */
  listWarrantyClaims() {
    return portalRequest<{ items: PortalWarrantyClaimStatus[] }>(
      "portal/warranty-claims",
    );
  },
  /** Warranties the signed-in user holds (TEC-191). */
  listWarranties(query: Record<string, string | number | undefined>) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") params.set(key, String(value));
    }
    const qs = params.toString();
    return portalRequest<PortalWarrantyPage>(
      `portal/warranties${qs ? `?${qs}` : ""}`,
    );
  },
  getWarranty(uuid: string) {
    return portalRequest<PortalWarranty>(
      `portal/warranties/${encodeURIComponent(uuid)}`,
    );
  },
  /** Vehicles the signed-in user owns (TEC-238). */
  listVehicles(limit: number, offset: number) {
    return portalRequest<PortalPage<PortalVehicle>>(
      `portal/vehicles?limit=${limit}&offset=${offset}`,
    );
  },
  getVehicle(uuid: string) {
    return portalRequest<PortalVehicleDetail>(
      `portal/vehicles/${encodeURIComponent(uuid)}`,
    );
  },
  /** One service of the signed-in user with parts and dealer (TEC-239). */
  getService(uuid: string) {
    return portalRequest<PortalService>(
      `portal/services/${encodeURIComponent(uuid)}`,
    );
  },
  /** Ownership transfer of the user's own vehicle (TEC-243). */
  listVehicleTransfers(vehicleUuid: string) {
    return portalRequest<{ items: PortalVehicleTransfer[] }>(
      `portal/vehicles/${encodeURIComponent(vehicleUuid)}/transfers`,
    );
  },
  startVehicleTransfer(vehicleUuid: string, phone: string) {
    return portalRequest<PortalVehicleTransfer>(
      `portal/vehicles/${encodeURIComponent(vehicleUuid)}/transfers`,
      { method: "POST", body: { phone } },
    );
  },
  verifyVehicleTransfer(
    transferUuid: string,
    body: PortalVehicleTransferVerifyInput,
  ) {
    return portalRequest<PortalVehicleTransfer>(
      `portal/vehicle-transfers/${encodeURIComponent(transferUuid)}/verify`,
      { method: "POST", body },
    );
  },
  cancelVehicleTransfer(transferUuid: string) {
    return portalRequest<PortalVehicleTransfer>(
      `portal/vehicle-transfers/${encodeURIComponent(transferUuid)}/cancel`,
      { method: "POST", body: {} },
    );
  },
  /** Signed vehicle intake contracts of the user (TEC-245). */
  listContracts(limit: number, offset: number) {
    return portalRequest<PortalPage<PortalContract>>(
      `portal/contracts?limit=${limit}&offset=${offset}`,
    );
  },
  /** Own notification preferences from the portal (TEC-245). */
  getNotificationPreferences() {
    return portalRequest<PortalNotificationPreferences>(
      "portal/notification-preferences",
    );
  },
  updateNotificationPreferences(body: PortalNotificationPreferences) {
    return portalRequest<PortalNotificationPreferences>(
      "portal/notification-preferences",
      { method: "PUT", body },
    );
  },
  /** Own appointments, upcoming (soonest first) or past (TEC-327). */
  listAppointments(
    period: PortalAppointmentPeriod,
    limit: number,
    offset: number,
  ) {
    return portalRequest<{ items: PortalAppointment[]; total: number }>(
      `portal/appointments?period=${period}&limit=${limit}&offset=${offset}`,
    );
  },
  createAppointment(body: PortalAppointmentInput) {
    return portalRequest<PortalAppointment>("portal/appointments", {
      method: "POST",
      body,
    });
  },
  cancelAppointment(uuid: string) {
    return portalRequest<PortalAppointment>(
      `portal/appointments/${encodeURIComponent(uuid)}/cancel`,
      { method: "POST", body: {} },
    );
  },
  /** Days and free slots of a dealer between two dates (inclusive). */
  dealerAvailability(dealerUuid: string, from: string, to: string) {
    return portalRequest<AppointmentAvailabilityDay[]>(
      `portal/dealers/${encodeURIComponent(dealerUuid)}/availability?from=${from}&to=${to}`,
    );
  },
  forgotPassword(email: string) {
    return portalRequest("auth/password/forgot", {
      method: "POST",
      body: { email },
    });
  },
  resetPassword(email: string, code: string, password: string) {
    return portalRequest("auth/password/reset", {
      method: "POST",
      body: { email, code, password },
    });
  },
};
