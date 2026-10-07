// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listFolders: vi.fn(),
  createFolder: vi.fn(),
  updateFolder: vi.fn(),
  deleteFolder: vi.fn(),
  listItems: vi.fn(),
  createItem: vi.fn(),
  updateItem: vi.fn(),
  archiveItem: vi.fn(),
  listVersions: vi.fn(),
  uploadVersion: vi.fn(),
  download: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("sonner", () => ({ toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: { dateTime: (value: string) => `dt(${value})` },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock(
  "@/features/library/services/library.service",
  async (importOriginal) => ({
    ...(await importOriginal<
      typeof import("@/features/library/services/library.service")
    >()),
    libraryService: api,
  }),
);

import { Permission } from "@/config/permissions";
import type {
  LibraryItem,
  LibraryListQuery,
  LibraryVersion,
} from "@/features/library/services/library.service";
import { LIBRARY_MAX_UPLOAD_BYTES } from "@/features/library/lib/library";
import { ApiError } from "@/lib/api/errors";

import { LibraryPage } from "./library-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => false,
})) as typeof window.matchMedia;
(
  globalThis as typeof globalThis & { ResizeObserver?: typeof ResizeObserver }
).ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as typeof ResizeObserver;

let container: HTMLDivElement;
let root: Root;

function version(patch: Partial<LibraryVersion> = {}): LibraryVersion {
  return {
    uuid: "v-tr-1",
    locale: "tr",
    version_no: 1,
    mime: "application/pdf",
    size_bytes: 2048,
    sha256: "a".repeat(64),
    created_at: "2026-10-01T08:00:00Z",
    ...patch,
  };
}

function item(patch: Partial<LibraryItem> = {}): LibraryItem {
  return {
    uuid: "i-1",
    folder_uuid: "f-1",
    name: "Install guide",
    description: "How to install",
    tags: ["guide"],
    access_level: "all_network",
    latest_version: version(),
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-02T08:00:00Z",
    ...patch,
  };
}

const VERSIONS = [
  version({ uuid: "v-tr-2", version_no: 2 }),
  version({ uuid: "v-tr-1", version_no: 1 }),
  version({ uuid: "v-de-1", locale: "de", version_no: 1 }),
];

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set([Permission.LibraryRead, Permission.LibraryManage]);
  api.listFolders.mockResolvedValue({
    folders: [
      {
        uuid: "f-1",
        parent_uuid: null,
        name: "Guides",
        sort_order: 0,
        created_at: "2026-10-01T08:00:00Z",
        updated_at: "2026-10-01T08:00:00Z",
      },
    ],
  });
  // latest_version follows the requested language (backend pickVersion).
  api.listItems.mockImplementation(async (params: LibraryListQuery) => {
    const latest =
      VERSIONS.find((v) => v.locale === params.locale) ?? VERSIONS[0];
    return {
      items: [item({ latest_version: latest })],
      total: 1,
      limit: 20,
      offset: 0,
    };
  });
  api.listVersions.mockResolvedValue({ versions: VERSIONS });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(LibraryPage, { slug: "acme" }),
      ),
    );
  });
  await flush();
}

function q<T extends Element = HTMLElement>(sel: string) {
  return document.body.querySelector(sel) as T | null;
}

async function click(sel: string) {
  const el = q(sel);
  if (!el) throw new Error(`missing ${sel}`);
  await act(async () => {
    el.click();
  });
  await flush();
}

async function selectValue(sel: string, value: string) {
  const el = q<HTMLSelectElement>(sel)!;
  const setter = Object.getOwnPropertyDescriptor(
    HTMLSelectElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

async function pickFile(sel: string, file: File) {
  const input = q<HTMLInputElement>(sel)!;
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [file],
  });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

function sizedFile(name: string, size: number) {
  const file = new File(["%PDF-1.7"], name, { type: "application/pdf" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

describe("LibraryPage", () => {
  it("lists items through the server list contract in the user's language", async () => {
    await renderPage();
    expect(api.listItems).toHaveBeenCalledWith({
      limit: 20,
      offset: 0,
      sort: "name",
      locale: "en",
    });
    expect(q("[data-testid=library-folder-tree]")?.textContent).toContain(
      "Guides",
    );
    expect(q<HTMLSelectElement>("#library-locale")?.value).toBe("en");
  });

  it("filters by folder", async () => {
    await renderPage();
    await click("[data-testid=library-folder][data-uuid=f-1]");
    expect(api.listItems).toHaveBeenLastCalledWith(
      expect.objectContaining({ folder: "f-1", offset: 0 }),
    );
  });

  it("lists the version of the selected language when the language changes", async () => {
    await renderPage();
    // "en" has no version: the list shows the tr fallback.
    expect(
      q("[data-testid=library-latest-version]")?.getAttribute("data-locale"),
    ).toBe("tr");

    await selectValue("#library-locale", "de");
    expect(api.listItems).toHaveBeenLastCalledWith(
      expect.objectContaining({ locale: "de" }),
    );
    expect(
      q("[data-testid=library-latest-version]")?.getAttribute("data-locale"),
    ).toBe("de");

    // The detail history shows only the selected language's versions.
    await click("button[aria-label='library.actions.view']");
    const history = () =>
      q("[data-testid=library-version-history]")?.textContent ?? "";
    expect(history()).toContain("v1");
    expect(history()).not.toContain("v2");

    await click("[data-testid=library-detail-languages] [data-locale=tr]");
    expect(history()).toContain("v2");
    expect(history()).toContain("v1");
    expect(api.listItems).toHaveBeenLastCalledWith(
      expect.objectContaining({ locale: "tr" }),
    );
  });

  it("downloads a version through a presigned URL", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    api.download.mockResolvedValue({
      url: "https://s3.example/file.pdf?sig=1",
      expires_at: "2026-10-07T10:00:00Z",
    });
    await renderPage();
    await click("button[aria-label='library.actions.download']");
    expect(api.download).toHaveBeenCalledWith("v-tr-2");
    expect(open).toHaveBeenCalledWith(
      "https://s3.example/file.pdf?sig=1",
      "_blank",
      "noopener,noreferrer",
    );
    open.mockRestore();
  });

  it("does not start an upload when the file is over 50 MB and shows an error", async () => {
    await renderPage();
    await click("button[aria-label='library.actions.upload']");
    expect(q("[data-testid=library-upload-dialog]")).not.toBeNull();

    await pickFile(
      "#library-upload-file",
      sizedFile("big.pdf", LIBRARY_MAX_UPLOAD_BYTES + 1),
    );
    const error = q("[data-testid=library-upload-error]");
    expect(error?.textContent).toContain("library.upload.too_large");
    const submit = q<HTMLButtonElement>("[data-testid=library-upload-submit]")!;
    expect(submit.disabled).toBe(true);

    // Even a forced submit does not reach the API.
    await act(async () => {
      submit.form!.requestSubmit();
    });
    await flush();
    expect(api.uploadVersion).not.toHaveBeenCalled();
    expect(q("[data-testid=library-upload-progress]")).toBeNull();
  });

  it("uploads a file within the limit in the chosen language with progress", async () => {
    let report: ((p: { loaded: number; total: number }) => void) | undefined;
    let finish: ((v: LibraryVersion) => void) | undefined;
    api.uploadVersion.mockImplementation(
      (_uuid: string, _input: unknown, onProgress) =>
        new Promise<LibraryVersion>((resolve) => {
          report = onProgress;
          finish = resolve;
        }),
    );
    await renderPage();
    await click("button[aria-label='library.actions.upload']");
    await selectValue("#library-upload-locale", "de");
    const file = sizedFile("guide.pdf", 1024);
    await pickFile("#library-upload-file", file);
    expect(q("[data-testid=library-upload-error]")).toBeNull();
    await click("[data-testid=library-upload-submit]");
    expect(api.uploadVersion).toHaveBeenCalledWith(
      "i-1",
      { file, locale: "de" },
      expect.any(Function),
    );
    await act(async () => report?.({ loaded: 512, total: 1024 }));
    expect(q("[data-testid=library-upload-progress]")?.textContent).toContain(
      '"percent":50',
    );
    await act(async () => finish?.(version({ locale: "de" })));
    await flush();
    expect(toast.success).toHaveBeenCalledWith("library.upload.done");
  });

  it("has no management buttons without library.manage", async () => {
    state.grants = new Set([Permission.LibraryRead]);
    await renderPage();
    expect(q("[data-testid=library-new-folder]")).toBeNull();
    expect(q("[data-testid=library-new-item]")).toBeNull();
    expect(q("[data-testid=library-folder-rename]")).toBeNull();
    expect(q("[data-testid=library-folder-delete]")).toBeNull();
    for (const action of ["edit", "upload", "archive"]) {
      expect(q(`button[aria-label='library.actions.${action}']`)).toBeNull();
    }
    // Readers still view and download.
    expect(q("button[aria-label='library.actions.view']")).not.toBeNull();
    expect(q("button[aria-label='library.actions.download']")).not.toBeNull();
    await click("button[aria-label='library.actions.view']");
    expect(q("[data-testid=library-detail-upload]")).toBeNull();
  });

  it("shows management buttons with library.manage", async () => {
    await renderPage();
    expect(q("[data-testid=library-new-folder]")).not.toBeNull();
    expect(q("[data-testid=library-new-item]")).not.toBeNull();
    expect(q("[data-testid=library-folder-delete]")).not.toBeNull();
    expect(q("button[aria-label='library.actions.archive']")).not.toBeNull();
  });

  it("explains the 409 when a non-empty folder is deleted", async () => {
    api.deleteFolder.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "LIBRARY_FOLDER_NOT_EMPTY",
        message: "Library folder is not empty",
      }),
    );
    await renderPage();
    await click("[data-testid=library-folder-delete]");
    await click("[data-testid=library-delete-folder-confirm]");
    expect(api.deleteFolder).toHaveBeenCalledWith("f-1");
    expect(q("[data-testid=library-folder-not-empty]")?.textContent).toBe(
      "library.folders.not_empty",
    );
    expect(toast.error).toHaveBeenCalledWith("library.folders.not_empty");
    // The dialog stays open with the explanation; the folder is still listed.
    expect(q("[data-testid=library-delete-folder-dialog]")).not.toBeNull();
    expect(q("[data-testid=library-folder][data-uuid=f-1]")).not.toBeNull();
  });

  it("creates an item with access level, role and tags", async () => {
    api.createItem.mockResolvedValue(item({ uuid: "i-2" }));
    await renderPage();
    await click("[data-testid=library-new-item]");
    const typeInto = async (sel: string, value: string) => {
      const el = q<HTMLInputElement>(sel)!;
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!;
      await act(async () => {
        setter.call(el, value);
        el.dispatchEvent(new Event("input", { bubbles: true }));
      });
    };
    await typeInto("#library-item-name", "Price list");
    await typeInto("#library-item-role", "dealer_owner");
    await typeInto("#library-item-tags", "price, 2026, price");
    await selectValue("#library-item-access", "dealers");
    await act(async () => {
      q<HTMLFormElement>(
        "[data-testid=library-item-dialog] form",
      )!.requestSubmit();
    });
    await flush();
    expect(api.createItem).toHaveBeenCalledWith({
      name: "Price list",
      description: null,
      folder_uuid: null,
      access_level: "dealers",
      role_slug: "dealer_owner",
      tags: ["price", "2026"],
    });
    // A new item goes straight to the version upload.
    expect(q("[data-testid=library-upload-dialog]")).not.toBeNull();
  });
});
