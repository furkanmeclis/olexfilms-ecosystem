// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listTypes: vi.fn(),
  createType: vi.fn(),
  listCertificates: vi.fn(),
  uploadCertificate: vi.fn(),
  listCategories: vi.fn(),
  listProducts: vi.fn(),
  canListTypes: true,
  isSuperAdmin: false,
  listUsers: vi.fn(),
}));
vi.mock("@/lib/auth/session-store", () => ({
  useAuthStore: (
    selector: (state: { user: { isSuperAdmin: boolean } }) => unknown,
  ) => selector({ user: { isSuperAdmin: api.isSuperAdmin } }),
}));
vi.mock("@/features/users/services/users.service", () => ({
  usersService: { list: api.listUsers },
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, locale: "en", format: {} }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => api.canListTypes }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/components/entity", () => ({
  EntityPage: ({
    children,
    actions,
  }: {
    children: ReactNode;
    actions: ReactNode;
  }) => (
    <main>
      {actions}
      {children}
    </main>
  ),
  EntityTable: () => null,
  EntityRowActions: () => null,
  useServerListState: () => ({ params: {}, tableState: {} }),
}));
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: api,
}));
vi.mock(
  "@/features/certificates/services/certificates.service",
  async (original) => ({
    ...(await original<object>()),
    certificatesService: api,
  }),
);

import { installRadixPolyfills, openPicker } from "@/test/form-controls";
import { CertificatesPage, CertificateTypesPage } from "./certificates-pages";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();
let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.canListTypes = true;
  api.isSuperAdmin = false;
  api.listUsers.mockResolvedValue({
    items: [
      {
        uuid: "staff-1",
        name: "Jane",
        surname: "Doe",
        email: "jane@example.com",
      },
    ],
  });
  api.listTypes.mockResolvedValue({
    items: [{ uuid: "type-1", name: { en: "Installer" } }],
    total: 1,
  });
  api.listCertificates.mockResolvedValue({ items: [], total: 0 });
  api.listCategories.mockResolvedValue({
    items: [{ uuid: "category-1", name: "Window films" }],
  });
  api.listProducts.mockImplementation(({ q }: { q: string }) =>
    Promise.resolve({
      items:
        q === "Other"
          ? [{ uuid: "product-2", name: "Other film" }]
          : [{ uuid: "product-1", name: "Solar film" }],
    }),
  );
  api.createType.mockResolvedValue({});
  api.uploadCertificate.mockResolvedValue({});
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});
async function tick(ms = 0) {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}
async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>{node}</QueryClientProvider>,
    );
  });
  await tick();
}
async function clickText(text: string) {
  const button = [...document.querySelectorAll("button")].find(
    (el) => el.textContent?.trim() === text,
  )!;
  expect(button).toBeTruthy();
  await act(async () => {
    button.click();
  });
  await tick();
}
async function type(selector: string, value: string) {
  const input = document.querySelector<HTMLInputElement>(selector)!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function pick(selector: string, text: string, search?: string) {
  await openPicker(document.querySelector(selector));
  await tick(350);
  if (search) {
    await type("[cmdk-input]", search);
    await tick(350);
  }
  const option = [...document.querySelectorAll('[role="option"]')].find((el) =>
    el.textContent?.includes(text),
  )!;
  expect(option).toBeTruthy();
  await act(async () => {
    (option as HTMLElement).click();
  });
  await tick();
}

it("searches catalog bindings, keeps labels across searches, removes selections, and submits UUID arrays", async () => {
  await render(<CertificateTypesPage />);
  await clickText("certificates.types.new");
  await type("#cert-name-tr", "Usta");
  await type("#cert-name-en", "Installer");
  await pick("#cert-cats", "Window films", "Window");
  await pick("#cert-products", "Solar film", "Solar");
  await pick("#cert-products", "Other film", "Other");
  expect(api.listCategories).toHaveBeenLastCalledWith({
    q: "Window",
    limit: 20,
    offset: 0,
  });
  expect(api.listProducts).toHaveBeenLastCalledWith({
    q: "Other",
    limit: 20,
    offset: 0,
  });
  expect(document.body.textContent).toContain("Solar film");
  expect(document.body.textContent).not.toContain("product-1");
  await act(async () => {
    document
      .querySelector<HTMLButtonElement>(
        '[aria-label="common.delete Other film"]',
      )!
      .click();
  });
  await clickText("common.save");
  expect(api.createType).toHaveBeenCalledWith(
    expect.objectContaining({
      category_uuids: ["category-1"],
      product_uuids: ["product-1"],
    }),
  );
});

it("submits the selected type UUID when the upload user can list types", async () => {
  api.isSuperAdmin = true;
  await render(<CertificatesPage />);
  await clickText("certificates.upload.title");
  await pick(
    '[role="combobox"][aria-label="certificates.fields.type"]',
    "Installer",
    "Install",
  );
  expect(api.listTypes).toHaveBeenLastCalledWith({
    q: "Install",
    limit: 20,
    offset: 0,
  });
  await pick('[role="combobox"]:not([aria-label])', "Jane Doe", "Jane");
  expect(api.listUsers).toHaveBeenLastCalledWith({
    q: "Jane",
    limit: 20,
    offset: 0,
  });
  await type('input[inputmode="numeric"]', "2026-10-08");
  const file = new File(["pdf"], "certificate.pdf", {
    type: "application/pdf",
  });
  const input = document.querySelector<HTMLInputElement>('input[type="file"]')!;
  await act(async () => {
    Object.defineProperty(input, "files", {
      value: [file],
      configurable: true,
    });
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await clickText("certificates.upload.submit");
  expect(api.uploadCertificate).toHaveBeenCalledWith({
    userUuid: "staff-1",
    typeUuid: "type-1",
    issuedAt: "2026-10-08",
    file,
  });
});

it("retains the type ID gap without requesting a directory the upload user cannot read", async () => {
  api.canListTypes = false;
  await render(<CertificatesPage />);
  await clickText("certificates.upload.title");
  expect(
    document.querySelector('input[aria-label="certificates.fields.type_uuid"]'),
  ).not.toBeNull();
  expect(api.listTypes).not.toHaveBeenCalled();
  expect(
    document.querySelector(
      'input[aria-label="certificates.fields.staff_uuid"]',
    ),
  ).not.toBeNull();
  expect(api.listUsers).not.toHaveBeenCalled();
});
