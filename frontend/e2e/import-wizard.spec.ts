import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG, type MockApi } from "./support/mock-api";

/**
 * TEC-758: import wizard v2 against a mocked BFF — file → columns →
 * preview (invalid row visible, card filter, error report) → confirm
 * (sentence with the counts, skipped rows acknowledgement) → result
 * (polled until applied) → undo (partially undone). The catalog product
 * import of the center answers with the rich staged preview of the stock
 * import (counts, row status, target, error code / field).
 */

const JOB = "0b9c4c1e-0000-4000-8000-000000000758";
const JOBS = `/v1/tenant/imports/${JOB}`;
const NOW = "2026-10-16T09:00:00Z";

const CSV = [
  "SKU,Name,Category,Raf",
  "PPF-GLOSS,Olex PPF Gloss,PPF,A-01",
  "PPF-MATTE,Olex PPF Matte,PPF,A-02",
  "PPF-NEW,Olex PPF New,PPF,A-03",
  "PPF-OLD,Olex PPF Old,PPF,A-04",
  "PPF-GLOSS,Olex PPF Gloss,PPF,A-01",
  "BAD-1,Bad Category,Nope,A-05",
].join("\n");

const target = (sku: string, name: string) => ({
  organization_uuid: "o1",
  organization_name: "Olex Merkez",
  product_sku: sku,
  product_name: name,
});

const previewSummary = {
  total: 6,
  valid: 4,
  invalid: 2,
  batch_uuid: "0b9c4c1e-0000-4000-8000-00000000b758",
  counts: { new: 3, link: 1, duplicate: 1, invalid: 1 },
  rows: [
    {
      index: 1,
      data: { sku: "PPF-GLOSS", name: "Olex PPF Gloss", category: "PPF" },
      status: "new",
      status_label: "New",
      target: target("PPF-GLOSS", "Olex PPF Gloss"),
    },
    {
      index: 2,
      data: { sku: "PPF-MATTE", name: "Olex PPF Matte", category: "PPF" },
      status: "new",
      status_label: "New",
      target: target("PPF-MATTE", "Olex PPF Matte"),
    },
    {
      index: 3,
      data: { sku: "PPF-NEW", name: "Olex PPF New", category: "PPF" },
      status: "new",
      status_label: "New",
      target: target("PPF-NEW", "Olex PPF New"),
    },
    {
      index: 4,
      data: { sku: "PPF-OLD", name: "Olex PPF Old", category: "PPF" },
      status: "link",
      status_label: "Link",
      target: target("PPF-OLD", "Olex PPF Old"),
    },
    {
      index: 5,
      data: { sku: "PPF-GLOSS", name: "Olex PPF Gloss", category: "PPF" },
      status: "duplicate",
      status_label: "Duplicate",
    },
    {
      index: 6,
      data: { sku: "BAD-1", name: "Bad Category", category: "Nope" },
      status: "invalid",
      status_label: "Invalid",
    },
  ],
  errors: [
    {
      index: 5,
      field: "sku",
      code: "IMPORT_DUPLICATE_IN_FILE",
      error: "The SKU appears twice in the file.",
    },
    {
      index: 6,
      field: "category",
      code: "IMPORT_CATEGORY_NOT_FOUND",
      error: "Category Nope was not found.",
    },
  ],
};

function job(status: string, extra: Record<string, unknown> = {}) {
  return {
    uuid: JOB,
    resource: "tenant.catalog.products",
    format: "csv",
    status,
    mapping: { SKU: "sku" },
    source_filename: "products.csv",
    created_at: NOW,
    ...extra,
  };
}

function asCenter(api: MockApi) {
  api.memberships = api.memberships.map((m) => ({
    ...m,
    type: "center",
    name: "Olex Merkez",
  }));
  api.permissions.push("catalog.write", "imports.tenant.read");
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
  // The revision-request widget (TEC-628) floats over the sheet footer.
  await context.route("https://issue.technowide.tr/**", (route) =>
    route.abort(),
  );
});

test("import wizard: preview errors, confirm counts, result and undo", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);
  let polls = 0;
  let rolledBack = false;
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === "/v1/catalog/products") {
      await ok({ items: [], total: 0, limit: 20, offset: 0 });
      return true;
    }
    if (method === "POST" && path === "/v1/catalog/products/import") {
      await ok(job("uploaded"), 201);
      return true;
    }
    if (method === "PATCH" && path === `${JOBS}/mapping`) {
      await ok(job("mapped"));
      return true;
    }
    if (method === "POST" && path === `${JOBS}/preview`) {
      await ok(job("previewed", { preview_summary: previewSummary }));
      return true;
    }
    if (method === "POST" && path === `${JOBS}/confirm`) {
      await ok(job("queued", { preview_summary: previewSummary }));
      return true;
    }
    if (method === "GET" && path === `${JOBS}/error-report`) {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type":
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
          "Content-Disposition": 'attachment; filename="import-errors.xlsx"',
        },
        body: Buffer.from("PK-xlsx"),
      });
      return true;
    }
    if (method === "GET" && path === JOBS) {
      polls++;
      if (rolledBack) {
        await ok(
          job("rolled_back", {
            preview_summary: {
              ...previewSummary,
              counts: { undone: 3, undo_rejected: 1, duplicate: 1, invalid: 1 },
            },
          }),
        );
      } else if (polls < 2) {
        await ok(job("applying", { preview_summary: previewSummary }));
      } else {
        await ok(
          job("applied", {
            applied_at: NOW,
            rollback_until: "2099-10-17T09:00:00Z",
            preview_summary: {
              ...previewSummary,
              counts: { applied: 4, duplicate: 1, invalid: 1 },
            },
          }),
        );
      }
      return true;
    }
    if (method === "POST" && path === `${JOBS}/rollback`) {
      rolledBack = true;
      await ok(
        job("rolled_back", {
          preview_summary: {
            ...previewSummary,
            counts: { undone: 3, undo_rejected: 1, duplicate: 1, invalid: 1 },
          },
        }),
      );
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/catalog/products`);
  await page.getByRole("button", { name: "Import", exact: true }).click();
  const wizard = page.getByTestId("import-wizard");
  await expect(wizard).toBeVisible();

  // File: drop zone + format from the extension.
  await expect(wizard.getByTestId("import-download-template")).toHaveText(
    "Download template (Excel)",
  );
  await wizard.locator("#import-file").setInputFiles({
    name: "products.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(CSV),
  });
  await expect(wizard.getByTestId("import-detected-format")).toHaveText("CSV");
  await wizard.getByTestId("import-next").click();

  // Columns: headers read in the browser, auto-matched, samples shown.
  const columns = wizard.getByTestId("import-columns");
  await expect(columns.locator('tr[data-field="category"]')).toContainText(
    "Category",
  );
  await expect(columns.locator('tr[data-field="sku"]')).toContainText(
    "PPF-GLOSS · PPF-MATTE · PPF-NEW",
  );
  await expect(columns.locator('tr[data-field="sku"]')).toContainText("Auto");
  await wizard.getByTestId("import-next").click();

  // Preview: cards from counts, the invalid row with its message and cell.
  await expect(wizard.getByTestId("import-status-card-new")).toContainText("3");
  await expect(wizard.getByTestId("import-status-card-invalid")).toContainText(
    "1",
  );
  await expect(wizard.getByText("Category Nope was not found.")).toBeVisible();
  await expect(wizard.locator('[data-error-cell="category"]')).toHaveText(
    "Nope",
  );
  await wizard.getByTestId("import-status-card-invalid").click();
  await expect(wizard.getByTestId("import-row-status")).toHaveCount(1);
  await expect(wizard.getByTestId("import-row-status")).toHaveText("Invalid");
  const download = page.waitForEvent("download");
  await wizard.getByTestId("import-error-report").click();
  expect((await download).suggestedFilename()).toBe("import-errors.xlsx");
  await wizard.getByTestId("import-next").click();

  // Confirm: one sentence with the counts; skipped rows need the checkbox.
  await expect(wizard.getByTestId("import-confirm-sentence")).toHaveText(
    "3 records will be created, 1 record will be updated. 2 rows will be skipped.",
  );
  const confirm = wizard.getByTestId("import-confirm");
  await expect(confirm).toHaveText("Import 4 rows");
  await expect(confirm).toBeDisabled();
  await wizard.getByTestId("import-ack-skipped").click();
  await expect(confirm).toBeEnabled();
  await confirm.click();

  // Result: polled until applied, link to the tenant job detail, undo.
  const result = wizard.getByTestId("import-result");
  await expect(result).toHaveAttribute("data-status", "applied");
  await expect(wizard.getByTestId("import-result-title")).toHaveText(
    "Import completed",
  );
  await expect(result).toContainText("Imported rows: 4, skipped rows: 2.");
  await expect(wizard.getByTestId("import-result-detail")).toHaveAttribute(
    "href",
    `/t/${SLUG}/settings/imports/${JOB}`,
  );
  await wizard.getByTestId("import-undo").click();
  await expect(result).toHaveAttribute("data-status", "rolled_back");
  await expect(wizard.getByTestId("import-result-title")).toHaveText(
    "The import was partially undone",
  );
  await expect(result).toContainText(
    "These rows were not undone because they moved after the import.",
  );

  expect(
    api.calls.filter((c) => c.startsWith(`POST ${JOBS}/confirm`)),
  ).toHaveLength(1);
});

test("error report button hides while the endpoint is missing", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);
  api.extra.push(async ({ method, path, ok }) => {
    if (method === "GET" && path === "/v1/catalog/products") {
      await ok({ items: [], total: 0, limit: 20, offset: 0 });
      return true;
    }
    if (method === "POST" && path === "/v1/catalog/products/import") {
      await ok(job("uploaded"), 201);
      return true;
    }
    if (method === "PATCH" && path === `${JOBS}/mapping`) {
      await ok(job("mapped"));
      return true;
    }
    if (method === "POST" && path === `${JOBS}/preview`) {
      await ok(job("previewed", { preview_summary: previewSummary }));
      return true;
    }
    return false; // error-report → 404 (not mocked)
  });

  await page.goto(`/t/${SLUG}/catalog/products`);
  await page.getByRole("button", { name: "Import", exact: true }).click();
  const wizard = page.getByTestId("import-wizard");
  await wizard.locator("#import-file").setInputFiles({
    name: "products.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(CSV),
  });
  await wizard.getByTestId("import-next").click();
  await wizard.getByTestId("import-next").click();
  await expect(wizard.getByTestId("import-preview-panel")).toBeVisible();
  await wizard.getByTestId("import-error-report").click();
  await expect(wizard.getByTestId("import-error-report")).toHaveCount(0);
});
