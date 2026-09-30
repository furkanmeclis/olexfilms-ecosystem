import { routes } from "@/config/routes";

export type NotificationActionKind = "download" | "route" | "external";

export type NotificationAction = {
  kind: NotificationActionKind;
  href: string;
  labelKey: "notifications.actions.download" | "notifications.actions.open";
};

export type NotificationActionContext = {
  tenantSlug?: string | null;
  payload?: unknown;
};

const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const EXPORT_DOWNLOAD = new RegExp(
  `^/v1/(platform|tenant)/exports/(${UUID})/download/?$`,
  "i",
);
const PLATFORM_API_ITEM = new RegExp(
  `^/v1/platform/([a-z0-9-]+)/(${UUID})(?:/.*)?$`,
  "i",
);
const TENANT_API_ITEM = new RegExp(
  `^/v1/tenant/([a-z0-9-]+)/(${UUID})(?:/.*)?$`,
  "i",
);

function payloadRecord(payload: unknown): Record<string, unknown> | null {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return null;
  }
  return payload as Record<string, unknown>;
}

function organizationSlugFrom(
  options?: NotificationActionContext,
): string | undefined {
  const fromCtx = options?.tenantSlug?.trim();
  if (fromCtx) return fromCtx;
  const slug = payloadRecord(options?.payload)?.organization_slug;
  return typeof slug === "string" && slug.trim() ? slug.trim() : undefined;
}

function platformRouteFor(resource: string, uuid: string): string | null {
  switch (resource) {
    case "exports":
      return routes.platform.exports.detail(uuid);
    case "imports":
      return routes.platform.imports.detail(uuid);
    case "notifications":
      return routes.platform.notifications.detail(uuid);
    case "users":
      return routes.platform.users.detail(uuid);
    case "roles":
      return routes.platform.roles.detail(uuid);
    default:
      return `/platform/${resource}/${uuid}`;
  }
}

function tenantRouteFor(
  resource: string,
  uuid: string,
  slug: string,
): string | null {
  switch (resource) {
    case "exports":
      return routes.tenant.exports.detail(slug, uuid);
    case "imports":
      return routes.tenant.imports.detail(slug, uuid);
    default:
      return null;
  }
}

function routeAction(href: string): NotificationAction {
  return { kind: "route", href, labelKey: "notifications.actions.open" };
}

/** Primary + optional companion actions for a notification `action_url`. */
export function notificationActions(
  actionUrl?: string | null,
  options?: NotificationActionContext,
): NotificationAction[] {
  if (!actionUrl) return [];

  const download = EXPORT_DOWNLOAD.exec(actionUrl);
  if (download?.[1] && download[2]) {
    const scope = download[1].toLowerCase();
    const uuid = download[2];
    const actions: NotificationAction[] = [
      {
        kind: "download",
        href: actionUrl,
        labelKey: "notifications.actions.download",
      },
    ];
    if (scope === "tenant") {
      const slug = organizationSlugFrom(options);
      if (slug) {
        actions.push(routeAction(routes.tenant.exports.detail(slug, uuid)));
      }
    } else {
      actions.push(routeAction(routes.platform.exports.detail(uuid)));
    }
    return actions;
  }

  if (actionUrl.startsWith("/v1/tenant/")) {
    const item = TENANT_API_ITEM.exec(actionUrl);
    const slug = organizationSlugFrom(options);
    if (item?.[1] && item[2] && slug) {
      const href = tenantRouteFor(item[1], item[2], slug);
      if (href) return [routeAction(href)];
    }
    return [];
  }

  if (actionUrl.startsWith("/v1/platform/")) {
    const item = PLATFORM_API_ITEM.exec(actionUrl);
    if (item?.[1] && item[2]) {
      const href = platformRouteFor(item[1], item[2]);
      if (href) return [routeAction(href)];
    }
    return [];
  }

  if (actionUrl.startsWith("/v1/")) {
    return [];
  }

  if (actionUrl.startsWith("/")) {
    return [routeAction(actionUrl)];
  }

  if (/^https?:\/\//i.test(actionUrl)) {
    return [
      {
        kind: "external",
        href: actionUrl,
        labelKey: "notifications.actions.open",
      },
    ];
  }

  return [];
}

export function notificationFingerprint(input: {
  uuid?: string | null;
  template_code?: string | null;
  title?: string | null;
  body?: string | null;
  action_url?: string | null;
}) {
  if (input.uuid) return input.uuid;
  const templateKey = [input.template_code, input.action_url]
    .map((value) => (value ?? "").trim())
    .filter(Boolean)
    .join("|");
  if (templateKey) return templateKey;
  return [input.template_code, input.title, input.body]
    .map((value) => (value ?? "").trim())
    .filter(Boolean)
    .join("|");
}
