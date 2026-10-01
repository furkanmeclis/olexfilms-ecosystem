/**
 * Session realms (TEC-90): cookie names and options of the two Auth.js
 * instances. No server-only imports here: auth.ts / auth-portal.ts (proxy
 * runtime) read these too. The panel (staff) and the customer / fleet portal
 * run two Auth.js instances with their own cookies, so one browser can hold
 * both sessions and signing out of one never touches the other. Every helper
 * here takes the realm explicitly: a realm-less call would write the panel
 * cookie with a portal token pair (or the other way round).
 */
export type AuthRealm = "panel" | "portal";

export const AUTH_REALMS: readonly AuthRealm[] = ["panel", "portal"];

/** Auth.js route prefix of each realm. */
export const AUTH_BASE_PATHS: Record<AuthRealm, string> = {
  panel: "/api/auth",
  portal: "/api/portal-auth",
};

/**
 * Auth.js picks `__Secure-` cookies from the site URL scheme, not NODE_ENV.
 * Follow the same rule so a production build served over plain http (local
 * prod compose, LAN preview) and an https dev tunnel both read and write the
 * cookie Auth.js actually set.
 */
export function secureCookies() {
  const url = process.env.AUTH_URL ?? process.env.NEXTAUTH_URL;
  if (url) return url.startsWith("https://");
  return process.env.NODE_ENV === "production";
}

function prefix() {
  return secureCookies() ? "__Secure-" : "";
}

/** Session cookie of a realm: `panel-session` / `portal-session`. */
export function sessionCookieName(realm: AuthRealm) {
  return `${prefix()}${realm}-session`;
}

/** Both spellings (http and https) of a realm's session cookie. */
export function sessionCookieBases(realm: AuthRealm) {
  return [`${realm}-session`, `__Secure-${realm}-session`] as const;
}

type CookieOptions = {
  httpOnly: boolean;
  secure: boolean;
  sameSite: "lax";
  path: string;
  maxAge?: number;
};

export function baseCookieOptions(): CookieOptions {
  return {
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax",
    path: "/",
  };
}

/**
 * Auth.js `cookies` option of a realm: session, csrf, callback, pkce, state,
 * nonce and webauthn challenge all carry the realm prefix so the two
 * instances never read or overwrite each other's cookies.
 */
export function authCookies(realm: AuthRealm) {
  const p = prefix();
  const opts = baseCookieOptions();
  const shortLived = { ...opts, maxAge: 60 * 15 };
  return {
    sessionToken: { name: sessionCookieName(realm), options: opts },
    callbackUrl: { name: `${p}${realm}.callback-url`, options: opts },
    csrfToken: { name: `${p}${realm}.csrf-token`, options: opts },
    pkceCodeVerifier: {
      name: `${p}${realm}.pkce.code_verifier`,
      options: shortLived,
    },
    state: { name: `${p}${realm}.state`, options: shortLived },
    nonce: { name: `${p}${realm}.nonce`, options: opts },
    webauthnChallenge: {
      name: `${p}${realm}.challenge`,
      options: shortLived,
    },
  };
}
