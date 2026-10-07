// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ModuleState } from "@/features/modules/types";

const captured = vi.hoisted(() => ({
  tables: [] as { columns: ColumnDef<ModuleState, unknown>[] }[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, locale: "en" }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => false }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "o", slug: "acme", type: "dealer" }),
}));
vi.mock("@/features/modules/hooks/use-features", async (orig) => ({
  ...(await orig<object>()),
  useFeatures: () => ({ data: { items: [] }, isLoading: false }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: { columns: ColumnDef<ModuleState, unknown>[] }) => {
    captured.tables.push(props);
    return null;
  },
}));

import { FeaturesPage } from "./features-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  captured.tables = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function moduleCell(source: string) {
  const col = captured.tables.at(-1)!.columns.find((c) => c.id === "module")!;
  const cell = col.cell as (ctx: unknown) => ReactElement;
  const host = document.createElement("div");
  const r = createRoot(host);
  act(() =>
    r.render(
      cell({
        row: { original: { key: "warranty", source, enabled: true } },
      }),
    ),
  );
  const html = host.innerHTML;
  act(() => r.unmount());
  return html;
}

describe("Özellikler: modules from a bundle subscription (TEC-311)", () => {
  it("labels source=service modules as on by subscription", async () => {
    await act(async () => {
      root.render(
        createElement(
          QueryClientProvider,
          { client: new QueryClient() },
          createElement(FeaturesPage, { slug: "acme" }),
        ),
      );
    });
    expect(moduleCell("service")).toContain("modules.via_subscription");
    expect(moduleCell("admin")).not.toContain("modules.via_subscription");
  });
});
