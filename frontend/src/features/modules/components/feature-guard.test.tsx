// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const list = vi.fn();
const request = vi.fn();

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string>) =>
      params?.module ? `${key}:${params.module}` : key,
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true, canAny: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "org-1", slug: "bayi-a" }),
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    list: () => list(),
    request: (key: string) => request(key),
  },
}));

import { FeatureGuard } from "./feature-guard";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

async function render(feature: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <FeatureGuard slug="bayi-a" feature={feature}>
          <p data-testid="page">module page</p>
        </FeatureGuard>
      </QueryClientProvider>,
    );
  });
  // Let the features query resolve.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  list.mockResolvedValue({
    organization_type: "dealer",
    items: [],
    enabled: ["organizations", "leads"],
  });
  request.mockResolvedValue({ status: "requested", recipients: 1 });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

describe("FeatureGuard", () => {
  it("renders the page when the module is on", async () => {
    await render("leads");
    expect(container.querySelector('[data-testid="page"]')).not.toBeNull();
    expect(
      container.querySelector('[data-testid="feature-disabled"]'),
    ).toBeNull();
  });

  it("blocks a module that is off when entered by URL and offers a request", async () => {
    await render("ai_assistant");
    expect(container.querySelector('[data-testid="page"]')).toBeNull();
    const blocked = container.querySelector('[data-testid="feature-disabled"]');
    expect(blocked).not.toBeNull();
    expect(blocked?.textContent).toContain("modules.guard.title");

    const button = [...container.querySelectorAll("button")].find((b) =>
      b.textContent?.includes("modules.request"),
    );
    expect(button).toBeDefined();
    await act(async () => {
      button?.click();
    });
    expect(request).toHaveBeenCalledWith("ai_assistant");
  });

  it("treats an unknown module key as off", async () => {
    await render("no_such_module");
    expect(
      container.querySelector('[data-testid="feature-disabled"]'),
    ).not.toBeNull();
  });
});
