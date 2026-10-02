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

/** Public warranty codes the upstream mock answers (keep in sync). */
export const E2E_WARRANTY = {
  ok: "E2EWARRANTY0001",
  limited: "E2ELIMITED00001",
  missing: "E2EMISSING00001",
};
