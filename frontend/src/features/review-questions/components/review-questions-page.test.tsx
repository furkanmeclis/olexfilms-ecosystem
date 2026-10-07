// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { ReviewQuestion } from "@/features/review-questions/services/review-questions.service";

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<ReviewQuestion>>[],
  dialogs: [] as {
    open: boolean;
    question: ReviewQuestion | null;
    onSubmit: (values: Record<string, unknown>) => Promise<unknown>;
  }[],
  api: {
    list: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    putLocale: vi.fn(),
  },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock(
  "@/features/review-questions/services/review-questions.service",
  async (orig) => ({
    ...(await orig<object>()),
    reviewQuestionsService: captured.api,
  }),
);
vi.mock(
  "@/features/review-questions/components/review-question-dialog",
  async (orig) => ({
    ...(await orig<object>()),
    ReviewQuestionDialog: (props: (typeof captured.dialogs)[number]) => {
      captured.dialogs.push(props);
      return null;
    },
  }),
);
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: React.ReactNode }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<ReviewQuestion>>) => {
    captured.tables.push(props);
    return null;
  },
}));

import { ReviewQuestionsPage } from "./review-questions-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function question(
  uuid: string,
  sort: number,
  patch: Partial<ReviewQuestion> = {},
): ReviewQuestion {
  return {
    uuid,
    question_key: `key_${uuid}`,
    question_type: "rating_1_5",
    target: "platform",
    is_required: false,
    is_active: true,
    sort_order: sort,
    locales: [
      { locale: "tr", text: `TR ${uuid}`, updated_at: "2026-10-01T00:00:00Z" },
      { locale: "en", text: `EN ${uuid}`, updated_at: "2026-10-01T00:00:00Z" },
    ],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    ...patch,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  captured.tables = [];
  captured.dialogs = [];
  for (const fn of Object.values(captured.api)) fn.mockReset();
  captured.api.list.mockResolvedValue({
    items: [question("a", 10), question("b", 20), question("c", 30)],
  });
  captured.api.update.mockImplementation(async (uuid: string, body) =>
    question(uuid, body.sort_order ?? 10, body),
  );
  captured.api.create.mockImplementation(async (body) =>
    question("new", body.sort_order, { ...body, locales: [] }),
  );
  captured.api.putLocale.mockResolvedValue({});
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ReviewQuestionsPage),
      ),
    );
  });
  await flush();
}

const lastTable = () => captured.tables[captured.tables.length - 1]!;
const lastDialog = () => captured.dialogs[captured.dialogs.length - 1]!;
const column = (id: string) =>
  (lastTable().columns as ColumnDef<ReviewQuestion, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ReviewQuestionsPage (TEC-353)", () => {
  it("is a client-side DataTable with filters, reorder and inline edit", async () => {
    await render();
    const table = lastTable();
    expect(captured.api.list).toHaveBeenCalled();
    expect(table.data).toHaveLength(3);
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.features).toMatchObject({
      persistKey: "platform-review-questions-v1",
      rowReorder: true,
      inlineEdit: true,
    });
    expect(column("question_type")?.meta).toMatchObject({
      filterVariant: "faceted",
    });
    expect(column("target")?.meta).toMatchObject({ filterVariant: "faceted" });
    expect(column("is_required")?.meta).toMatchObject({
      filterVariant: "boolean",
      editVariant: "boolean",
    });
    expect(column("is_active")?.meta).toMatchObject({
      filterVariant: "boolean",
      editVariant: "boolean",
    });
  });

  it("saves the order after a drag with sort_order PATCHes of moved rows", async () => {
    await render();
    const rows = lastTable().data as ReviewQuestion[];
    // Drag "c" to the top: c, a, b.
    await act(async () => {
      lastTable().onRowReorder?.([rows[2]!, rows[0]!, rows[1]!]);
    });
    await flush();
    expect(captured.api.update.mock.calls).toEqual([
      ["c", { sort_order: 10 }],
      ["a", { sort_order: 20 }],
      ["b", { sort_order: 30 }],
    ]);
  });

  it("sends only the rows whose order changed", async () => {
    await render();
    const rows = lastTable().data as ReviewQuestion[];
    // Swap b and c: a keeps 10.
    await act(async () => {
      lastTable().onRowReorder?.([rows[0]!, rows[2]!, rows[1]!]);
    });
    await flush();
    expect(captured.api.update.mock.calls).toEqual([
      ["c", { sort_order: 20 }],
      ["b", { sort_order: 30 }],
    ]);
  });

  it("toggles required / active inline with a single-field PATCH", async () => {
    await render();
    const rows = lastTable().data as ReviewQuestion[];
    await act(async () => {
      lastTable().onCellEdit?.({
        row: rows[1]!,
        rowId: "b",
        columnId: "is_required",
        value: true,
      });
    });
    await flush();
    expect(captured.api.update).toHaveBeenCalledWith("b", {
      is_required: true,
    });
  });

  it("creates a question at the end and saves its language texts", async () => {
    await render();
    await act(async () => {
      await lastDialog().onSubmit({
        question_key: " staff ",
        question_type: "text",
        target: "product",
        is_required: true,
        is_active: true,
        text_tr: "Personel nasıldı?",
        text_en: " How was the staff? ",
        text_zh_CN: "",
      });
    });
    await flush();
    expect(captured.api.create).toHaveBeenCalledWith({
      question_key: "staff",
      question_type: "text",
      target: "product",
      is_required: true,
      is_active: true,
      sort_order: 40,
    });
    expect(captured.api.putLocale.mock.calls).toEqual([
      ["new", "tr", "Personel nasıldı?"],
      ["new", "en", "How was the staff?"],
    ]);
  });
});
