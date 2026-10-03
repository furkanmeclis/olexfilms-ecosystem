import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the center tasks e2e (TEC-221): a center member with
 * tasks.read + tasks.write, two distributors/dealers to pick as the
 * subject, two center members as assignees and an in-memory task list with
 * comments. Filters of GET /v1/tasks are applied like the API does.
 */

export const TASK_SLUG = "olex-merkez";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
const USER = "0b9c4c1e-0000-4000-8000-000000000002";
export const OTHER_MEMBER = "0b9c4c1e-0000-4000-8000-000000000103";
export const DEALER = "0b9c4c1e-0000-4000-8000-000000000104";
const DISTRIBUTOR = "0b9c4c1e-0000-4000-8000-000000000105";
const NOW = "2026-10-01T09:00:00Z";

const PERMISSIONS = ["tasks.read", "tasks.write", "organizations.read"];

type Json = Record<string, unknown>;
type Task = Json & { uuid: string; status: string; priority: string };

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

const members = [
  { uuid: USER, name: "E2E Merkez" },
  { uuid: OTHER_MEMBER, name: "Ayşe Merkez" },
];
const subjects = [
  { uuid: DEALER, name: "Kuzey Oto", type: "dealer" },
  { uuid: DISTRIBUTOR, name: "Ege Dağıtım", type: "distributor" },
];

function seedTask(n: number, patch: Partial<Task>): Task {
  return {
    uuid: `0b9c4c1e-0000-4000-8000-00000000020${n}`,
    title: `Task ${n}`,
    description: "",
    subject_organization: subjects[0],
    assignee: members[1],
    created_by: members[0],
    priority: "normal",
    status: "open",
    source: "manual",
    due_at: null,
    closed_at: null,
    comment_count: 0,
    created_at: NOW,
    updated_at: NOW,
    ...patch,
  };
}

export class TaskMock {
  tasks: Task[] = [
    seedTask(1, { title: "Monthly visit", priority: "urgent" }),
    seedTask(2, {
      title: "Overdue price review",
      due_at: "2020-01-01T09:00:00Z",
      subject_organization: subjects[1],
    }),
    seedTask(3, { title: "Closed follow-up", status: "done", closed_at: NOW }),
  ];
  comments: Record<string, Json[]> = {};
  calls: string[] = [];
  bodies: Json[] = [];
  unknown: string[] = [];
  private seq = 10;

  private membership() {
    return {
      uuid: ORG,
      slug: TASK_SLUG,
      name: "Olex Merkez",
      role: "owner",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: "center",
      brand: { slug: "olex", name: "Olex" },
      parent: null,
    };
  }

  private me() {
    return {
      effective_locale: "en",
      effective_timezone: "Europe/Istanbul",
      user: {
        uuid: USER,
        email: "e2e@example.com",
        name: "E2E",
        surname: "Merkez",
        status: "active",
        is_super_admin: false,
        email_verified: true,
        locale: "en",
        timezone: "Europe/Istanbul",
      },
      roles: [],
      permissions: PERMISSIONS,
      grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "brand"])),
      active_organization_uuid: ORG,
      organization_roles: ["owner"],
      organizations: [this.membership()],
      links: {
        profile: "/v1/auth/profile",
        change_password: "/v1/auth/password/change",
        notification_preferences: "/v1/notification-preferences",
      },
      channels: { user: `user:${USER}` },
      realtime: { enabled: false, user_channel: `user:${USER}` },
    };
  }

  private filter(url: URL): Task[] {
    const p = url.searchParams;
    const status = p.get("status");
    const priority = p.get("priority");
    const assignee = p.get("assignee_user_uuid");
    const subject = p.get("subject_organization_uuid");
    const before = p.get("due_before");
    const after = p.get("due_after");
    return this.tasks.filter((t) => {
      if (status === "active" && !["open", "in_progress"].includes(t.status))
        return false;
      if (status && status !== "active" && t.status !== status) return false;
      if (priority && t.priority !== priority) return false;
      if (assignee && (t.assignee as Json | null)?.uuid !== assignee)
        return false;
      if (subject && (t.subject_organization as Json).uuid !== subject)
        return false;
      const due = t.due_at ? new Date(String(t.due_at)).getTime() : null;
      if ((before || after) && due === null) return false;
      if (before && due! >= new Date(before).getTime()) return false;
      if (after && due! < new Date(after).getTime()) return false;
      return true;
    });
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    this.calls.push(`${method} ${path}${url.search}`);
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const page = (items: unknown[]) =>
      ok({ items, total: items.length, limit: 20, offset: 0 });
    const body = (): Json => {
      const b = (req.postDataJSON() ?? {}) as Json;
      this.bodies.push(b);
      return b;
    };

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${TASK_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: TASK_SLUG,
        name: "Olex Merkez",
        status: "active",
        logo_url: null,
        access_ok: true,
      });
    }
    if (method === "GET" && path === "/v1/auth/me") return ok(this.me());
    if (method === "GET" && path === "/v1/auth/step-up") {
      return ok({ valid: false, expires_at: null, methods: [] });
    }
    if (method === "GET" && path === "/v1/features") {
      return ok({ organization_type: "center", items: [], enabled: ["tasks"] });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === "/v1/tenant/organizations") {
      return ok({ items: subjects, scope: "brand" });
    }
    if (method === "GET" && path === "/v1/tasks/assignees") {
      return ok({ items: members });
    }
    if (method === "GET" && path === "/v1/tasks") {
      return page(this.filter(url));
    }
    if (method === "POST" && path === "/v1/tasks") {
      const b = body();
      const task = seedTask(0, {
        uuid: `0b9c4c1e-0000-4000-8000-0000000003${this.seq++}`,
        title: String(b.title),
        description: String(b.description ?? ""),
        subject_organization:
          subjects.find((s) => s.uuid === b.subject_organization_uuid) ??
          subjects[0],
        assignee: members.find((m) => m.uuid === b.assignee_user_uuid) ?? null,
        priority: String(b.priority ?? "normal"),
        due_at: (b.due_at as string | undefined) ?? null,
      });
      this.tasks.unshift(task);
      return ok(task, 201);
    }
    const one = path.match(/^\/v1\/tasks\/([^/]+)$/);
    const task = one ? this.tasks.find((t) => t.uuid === one[1]) : undefined;
    if (one && !task) {
      return route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Task not found" } },
      });
    }
    if (method === "GET" && task) return ok(task);
    if (method === "PATCH" && task) {
      const b = body();
      if ("assignee_user_uuid" in b) {
        task.assignee =
          members.find((m) => m.uuid === b.assignee_user_uuid) ?? null;
      }
      for (const k of [
        "title",
        "description",
        "priority",
        "status",
        "due_at",
      ]) {
        if (k in b) task[k] = b[k];
      }
      return ok(task);
    }
    const comments = path.match(/^\/v1\/tasks\/([^/]+)\/comments$/);
    if (comments && method === "GET") {
      return page(this.comments[comments[1]] ?? []);
    }
    if (comments && method === "POST") {
      const b = body();
      const c = {
        uuid: `0b9c4c1e-0000-4000-8000-0000000004${this.seq++}`,
        body: String(b.body),
        author: members[0],
        created_at: NOW,
      };
      (this.comments[comments[1]] ??= []).push(c);
      return ok(c, 201);
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the task mock. */
export async function mockTasks(page: Page): Promise<TaskMock> {
  const api = new TaskMock();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
