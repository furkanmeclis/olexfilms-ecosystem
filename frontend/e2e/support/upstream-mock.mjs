// Mocked Go API for the server side of the e2e build (TEC-218).
//
// `page.route` only sees browser traffic. Three flows call Go from the
// Next.js server instead: the Auth.js credentials login (auth/login + the
// adapter's user lookup), the public warranty page (/garanti/{code},
// rendered on the server) and the portal phone OTP sign-in
// (auth/otp/verify, TEC-246). The organization switch also goes through
// the real BFF so it can rewrite the session cookie. This tiny server
// answers exactly those; every other path drops the connection, so the
// rest of the build still fails fast as it did with the closed-port
// API_URL.
//
// Fixtures must match e2e/support/mock-api.ts (USER, ORG) and the specs.
import { createServer } from "node:http";

const PORT = Number(process.env.E2E_UPSTREAM_PORT ?? 3184);
const ADAPTER_KEY = process.env.AUTH_ADAPTER_SECRET ?? "";

const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
/** Memberships of the login spec by slug (organization switcher). */
const ORGS = { acme: ORG, beta: "0b9c4c1e-0000-4000-8000-000000000003" };
const LOGIN = { email: "e2e@example.com", password: "e2e-password-1" };

/**
 * Portal OTP sign-in (TEC-246): the code the fake WhatsApp sender of the
 * portal spec delivers per E.164 phone and the portal user it signs in.
 * Keep in sync with E2E_PORTAL in e2e/support/constants.ts.
 */
const PORTAL_OTP = {
  "+905551110001": {
    code: "246001",
    sub: "0b9c4c1e-0000-4000-8000-000000000246",
  },
  "+905551110002": {
    code: "246002",
    sub: "0b9c4c1e-0000-4000-8000-000000000247",
  },
};

/** TR national digits as typed ("555 111 00 01", "0555...") to E.164. */
function toE164(phone, country) {
  const digits = String(phone ?? "").replace(/\D/g, "");
  if (
    String(phone ?? "")
      .trim()
      .startsWith("+")
  )
    return `+${digits}`;
  if (country && country !== "TR") return `+${digits}`;
  return `+90${digits.replace(/^0/, "").replace(/^90(?=\d{10}$)/, "")}`;
}

/** Public codes of the warranty spec: one per screen. */
const WARRANTY_OK = "E2EWARRANTY0001";
const WARRANTY_LIMITED = "E2ELIMITED00001";

const warranty = {
  public_code: WARRANTY_OK,
  status: "active",
  start_at: "2026-01-15T10:00:00Z",
  end_at: "2036-01-15T20:59:59Z",
  days_remaining: 3393,
  product: { name: "Olex PPF Gloss" },
  brand: { name: "Olexfilms", slug: "olex" },
  dealer: { name: "Kadıköy Bayi", city: "İstanbul" },
  vehicle: {
    brand_name: "BMW",
    brand_logo_uuid: null,
    model_name: "M3",
    model_year: 2024,
    plate_masked: "34 *** 12",
    vin_last4: "6752",
  },
};

const b64url = (v) => Buffer.from(JSON.stringify(v)).toString("base64url");
/** Unsigned access token: the frontend only reads its claims (oid, aud). */
const accessToken = (oid = ORG, sub = USER) =>
  `${b64url({ alg: "none", typ: "JWT" })}.${b64url({
    sub,
    oid,
    exp: Math.floor(Date.now() / 1000) + 3600,
  })}.e2e`;

/** Upstream calls seen, newest last: "METHOD /path" (GET /__calls). */
const calls = [];

function send(res, status, body, headers = {}) {
  res.writeHead(status, { "Content-Type": "application/json", ...headers });
  res.end(JSON.stringify(body));
}

const error = (code, message) => ({ success: false, error: { code, message } });

async function readJson(req) {
  let raw = "";
  for await (const chunk of req) raw += chunk;
  try {
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${PORT}`);
  const path = url.pathname;

  if (path === "/health") return send(res, 200, { ok: true });
  if (path === "/__calls") return send(res, 200, calls);
  calls.push(`${req.method} ${path}`);

  if (req.method === "POST" && path === "/v1/auth/login") {
    const body = await readJson(req);
    if (body.email !== LOGIN.email || body.password !== LOGIN.password) {
      return send(
        res,
        401,
        error("INVALID_CREDENTIALS", "Invalid email or password"),
      );
    }
    return send(res, 200, {
      success: true,
      data: {
        access_token: accessToken(),
        refresh_token: "e2e-refresh",
        expires_in: 3600,
      },
    });
  }

  // Portal phone OTP (TEC-246): the code the fake WhatsApp sender delivered.
  if (req.method === "POST" && path === "/v1/auth/otp/verify") {
    const body = await readJson(req);
    const user = PORTAL_OTP[toE164(body.phone, body.country)];
    if (body.purpose !== "customer_login" || !user || body.code !== user.code) {
      return send(res, 401, error("INVALID_OTP_CODE", "Invalid code"));
    }
    return send(res, 200, {
      success: true,
      data: {
        access_token: accessToken(null, user.sub),
        refresh_token: "e2e-portal-refresh",
        expires_in: 3600,
      },
    });
  }

  if (req.method === "GET" && path === "/v1/internal/auth/users/by-email") {
    if (req.headers["x-auth-adapter-key"] !== ADAPTER_KEY) {
      return send(res, 401, error("UNAUTHORIZED", "Bad adapter key"));
    }
    return send(res, 200, {
      success: true,
      data: { id: USER, email: LOGIN.email, name: "E2E Dealer" },
    });
  }

  // The panel BFF persists the returned pair in the session cookie, so the
  // switched organization (oid) is what the next request carries.
  if (req.method === "POST" && path === "/v1/auth/organization-context") {
    const body = await readJson(req);
    const oid = ORGS[body.organization_slug];
    if (!req.headers.authorization?.startsWith("Bearer ")) {
      return send(res, 401, error("UNAUTHORIZED", "No token"));
    }
    if (!oid) return send(res, 403, error("FORBIDDEN", "No membership"));
    return send(res, 200, {
      success: true,
      data: {
        authenticated: true,
        access_token: accessToken(oid),
        refresh_token: "e2e-refresh",
        expires_in: 3600,
      },
    });
  }

  const code = path.match(/^\/v1\/public\/warranties\/([^/]+)$/)?.[1];
  if (req.method === "GET" && code) {
    if (code === WARRANTY_OK) {
      return send(res, 200, { success: true, data: warranty });
    }
    if (code === WARRANTY_LIMITED) {
      return send(res, 429, error("RATE_LIMITED", "Too many requests"), {
        "Retry-After": "42",
      });
    }
    return send(res, 404, error("NOT_FOUND", "Warranty not found"));
  }

  // Not mocked: behave like the closed port the other specs rely on.
  req.socket.destroy();
});

server.listen(PORT, "127.0.0.1");
