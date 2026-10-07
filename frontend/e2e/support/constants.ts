/** Auth.js secret of the mocked e2e server only (never a real secret). */
export const E2E_AUTH_SECRET = "e2e-mocked-auth-secret-not-for-production-use";

/** Port of the mocked Go API (e2e/support/upstream-mock.mjs, TEC-218). */
export const E2E_UPSTREAM_PORT = Number(process.env.E2E_UPSTREAM_PORT ?? 3184);

/** Adapter key shared by the e2e server and the upstream mock. */
export const E2E_ADAPTER_SECRET = "e2e-adapter-key-not-for-production-use";

/** Credentials the upstream mock accepts (keep in sync with it). */
export const E2E_LOGIN = {
  email: "e2e@example.com",
  password: "e2e-password-1",
};

/**
 * Portal phone OTP accounts of the upstream mock (TEC-246, keep in sync
 * with PORTAL_OTP there): the code the fake WhatsApp sender delivers and
 * the portal user (token subject) it signs in.
 */
export const E2E_PORTAL = {
  owner: {
    phone: "+905551110001",
    typed: "555 111 00 01",
    code: "246001",
    uuid: "0b9c4c1e-0000-4000-8000-000000000246",
    name: "Ayşe",
    surname: "Yılmaz",
  },
  buyer: {
    phone: "+905551110002",
    typed: "555 111 00 02",
    code: "246002",
    uuid: "0b9c4c1e-0000-4000-8000-000000000247",
    name: "Mehmet",
    surname: "Demir",
  },
} as const;

/** Public warranty codes the upstream mock answers (keep in sync). */
export const E2E_WARRANTY = {
  ok: "E2EWARRANTY0001",
  limited: "E2ELIMITED00001",
  missing: "E2EMISSING00001",
};

/** Public quote token the upstream mock answers (TEC-320, keep in sync). */
export const E2E_QUOTE = {
  ok: "3f1c2b7a-8d4e-4b6f-9a1c-2e3d4f5a6b7c",
  missing: "00000000-0000-4000-8000-000000000320",
};
