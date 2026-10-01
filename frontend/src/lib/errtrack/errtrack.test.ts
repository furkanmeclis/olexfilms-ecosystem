import type { ErrorEvent } from "@sentry/nextjs";
import { describe, expect, it } from "vitest";

import { MODULES, moduleFromPath, parseModule } from "./modules";
import {
  baseOptions,
  beforeSendFor,
  browserOptions,
  TUNNEL_PATH,
} from "./options";
import { MASK_FILTERED, scrubEvent, scrubString } from "./scrub";
import { envelopeUrl, parseDsn, resolveTunnelTarget } from "./tunnel";

const DSN = "https://0123456789abcdef0123456789abcdef@errors.technowide.net/42";

describe("modules", () => {
  it("is the fixed list of 14", () => {
    expect(MODULES).toEqual([
      "auth",
      "org",
      "catalog",
      "inventory",
      "order",
      "service",
      "warranty",
      "accounting",
      "notification",
      "whatsapp",
      "ai",
      "mcp",
      "migrator",
      "docs",
    ]);
  });

  it("maps unknown modules to unknown", () => {
    for (const v of ["", "billing", "Unknown", "settings", undefined, null]) {
      expect(parseModule(v)).toBe("unknown");
    }
    expect(parseModule(" Order ")).toBe("order");
  });

  it("derives the module from paths", () => {
    expect(moduleFromPath("/platform/organizations/x")).toBe("org");
    expect(moduleFromPath("/platform/users")).toBe("auth");
    expect(moduleFromPath("/api/v1/tenant/orders/5?x=1")).toBe("order");
    expect(moduleFromPath("/t/bayi-a/warranties")).toBe("warranty");
    expect(moduleFromPath("/garanti/ABC123")).toBe("warranty");
    expect(moduleFromPath("/platform/storage")).toBe("unknown");
    expect(moduleFromPath("/")).toBe("unknown");
    expect(moduleFromPath(undefined)).toBe("unknown");
  });
});

describe("scrub", () => {
  it("masks e-mails and phones, keeps ids and times", () => {
    expect(scrubString("mail ali.veli+x@olexfilms.app failed")).toBe(
      "mail [email] failed",
    );
    expect(scrubString("phone +905551234567 invalid")).toBe(
      "phone [phone] invalid",
    );
    expect(scrubString("call 0555 123 45 67 now")).toBe("call [phone] now");
    expect(scrubString("order 550e8400-e29b-41d4-a716-446655440000")).toBe(
      "order 550e8400-e29b-41d4-a716-446655440000",
    );
    expect(scrubString("at 2026-10-01 12:00:00")).toBe(
      "at 2026-10-01 12:00:00",
    );
    expect(scrubString("id 12345")).toBe("id 12345");
  });

  it("scrubs a whole event; user keeps only id", () => {
    const event = {
      message: "user ali@example.com",
      server_name: "host-1",
      user: {
        id: "u-1",
        email: "ali@example.com",
        username: "ali",
        ip_address: "1.2.3.4",
        name: "Ali Veli",
      },
      exception: {
        values: [{ type: "Error", value: "sms to +905551234567 failed" }],
      },
      breadcrumbs: [
        {
          message: "login ali@example.com",
          data: { url: "/api/v1/x?phone=1", name: "Ali" },
        },
      ],
      tags: { module: "auth", email: "a@b.co" },
      extra: { customer: { full_name: "Ali Veli", city: "Ankara" } },
      contexts: { runtime: { name: "node" }, form: { phone: "+905551234567" } },
      request: {
        method: "POST",
        url: "https://olexfilms.app/portal?email=a@b.co",
        cookies: { session: "x" },
        headers: { authorization: "Bearer x" },
        data: "phone=+905551234567",
        query_string: "email=a@b.co",
      },
    } as unknown as ErrorEvent;

    const out = scrubEvent(event);
    expect(out.user).toEqual({ id: "u-1" });
    expect(out.server_name).toBeUndefined();
    expect(out.message).toBe("user [email]");
    expect(out.exception?.values?.[0]?.value).toBe("sms to [phone] failed");
    expect(out.breadcrumbs?.[0]?.message).toBe("login [email]");
    expect(out.breadcrumbs?.[0]?.data).toEqual({
      url: "/api/v1/x",
      name: MASK_FILTERED,
    });
    expect(out.tags).toEqual({ module: "auth", email: MASK_FILTERED });
    expect(out.extra).toEqual({
      customer: { full_name: MASK_FILTERED, city: "Ankara" },
    });
    expect(out.contexts?.runtime).toEqual({ name: "node" });
    expect(out.contexts?.form).toEqual({ phone: MASK_FILTERED });
    expect(out.request).toEqual({
      method: "POST",
      url: "https://olexfilms.app/portal",
    });
    expect(JSON.stringify(out)).not.toMatch(/ali@example|5551234567|Ali Veli/);
  });

  it("drops the user when it has no id", () => {
    const out = scrubEvent({
      user: { email: "a@b.co" },
    } as unknown as ErrorEvent);
    expect(out.user).toBeUndefined();
  });
});

describe("options", () => {
  it("is disabled (no-op) without a DSN, never sends PII", () => {
    const o = baseOptions({
      dsn: "  ",
      environment: undefined,
      release: undefined,
      runtime: "nodejs",
    });
    expect(o.enabled).toBe(false);
    expect(o.dsn).toBeUndefined();
    expect(o.sendDefaultPii).toBe(false);
    expect(o.tracesSampleRate).toBe(0);
  });

  it("browser options use the same-origin tunnel", () => {
    expect(browserOptions().tunnel).toBe(TUNNEL_PATH);
    expect(TUNNEL_PATH).toBe("/api/monitoring");
  });

  it("beforeSend tags the module from the URL and scrubs", () => {
    const send = beforeSendFor("nodejs");
    const out = send({
      request: { url: "https://olexfilms.app/platform/organizations?q=a@b.co" },
      exception: { values: [{ value: "boom a@b.co" }] },
    } as unknown as ErrorEvent);
    expect(out.tags).toMatchObject({ module: "org", runtime: "nodejs" });
    expect(out.exception?.values?.[0]?.value).toBe("boom [email]");
    expect(out.request?.url).toBe(
      "https://olexfilms.app/platform/organizations",
    );
  });

  it("normalizes an explicit unknown module tag", () => {
    const out = beforeSendFor("browser")({
      tags: { module: "crm" },
    } as unknown as ErrorEvent);
    expect(out.tags?.module).toBe("unknown");
  });
});

describe("tunnel", () => {
  const envelope = (dsn: string) =>
    `${JSON.stringify({ dsn, event_id: "x" })}\n{"type":"event"}\n{}`;

  it("forwards only the configured DSN", () => {
    expect(resolveTunnelTarget(envelope(DSN), DSN)).toBe(
      "https://errors.technowide.net/api/42/envelope/?sentry_key=0123456789abcdef0123456789abcdef",
    );
    expect(
      resolveTunnelTarget(envelope(DSN.replace("/42", "/43")), DSN),
    ).toBeNull();
    expect(
      resolveTunnelTarget(
        envelope(DSN.replace("errors.technowide.net", "evil.example")),
        DSN,
      ),
    ).toBeNull();
    expect(resolveTunnelTarget("not json\n{}", DSN)).toBeNull();
    expect(resolveTunnelTarget(envelope(DSN), undefined)).toBeNull();
  });

  it("keeps a DSN path prefix", () => {
    const dsn = "https://key@host.example/errors/7";
    const parsed = parseDsn(dsn);
    expect(parsed).not.toBeNull();
    expect(envelopeUrl(parsed!)).toBe(
      "https://host.example/errors/api/7/envelope/?sentry_key=key",
    );
  });
});
