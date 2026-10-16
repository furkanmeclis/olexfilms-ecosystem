import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import {
  mockWarehouse,
  PRODUCT,
  WAREHOUSE_SLUG,
  type WarehouseMock,
} from "./support/warehouse-mock";

/**
 * TEC-759: Warehouse > Stock entries "Import from Excel" against a mocked
 * BFF — the center downloads the template, uploads a file, sees one
 * duplicate and one invalid row in the preview, confirms with the stock
 * sentence, opens the import entry from the result and undoes the import
 * (partially: one unit moved). A distributor does not get the button.
 */

const JOB = "0b9c4c1e-0000-4000-8000-000000000759";
const JOBS = `/v1/tenant/imports/${JOB}`;
const ENTRY = "0b9c4c1e-0000-4000-8000-0000000e0759";
const NOW = "2026-10-16T09:00:00Z";
const base = `/t/${WAREHOUSE_SLUG}/warehouse`;

const CSV = [
  "barcode,product_sku,location_code",
  "OLX-000001,PPF-190,A-01",
  "OLX-000002,PPF-190,",
  "OLX-000001,PPF-190,A-01",
  "OLX-000003,NOPE,",
].join("\n");

const target = (location?: string) => ({
  organization_uuid: "o1",
  organization_name: "Olex Merkez",
  product_sku: PRODUCT.sku,
  product_name: PRODUCT.name,
  ...(location
    ? { owner_type: "warehouse_location", location_code: location }
    : {}),
});

const previewSummary = {
  total: 4,
  valid: 2,
  invalid: 2,
  batch_uuid: JOB,
  counts: {
    new: 2,
    duplicate: 1,
    invalid: 1,
    conflict: 0,
    applied: 0,
    undone: 0,
    undo_rejected: 0,
  },
  rows: [
    {
      index: 1,
      data: {
        barcode: "OLX-000001",
        product_sku: "PPF-190",
        location_code: "A-01",
      },
      status: "new",
      status_label: "New",
      target: target("A-01"),
    },
    {
      index: 2,
      data: { barcode: "OLX-000002", product_sku: "PPF-190" },
      status: "new",
      status_label: "New",
      target: target(),
    },
    {
      index: 3,
      data: {
        barcode: "OLX-000001",
        product_sku: "PPF-190",
        location_code: "A-01",
      },
      status: "duplicate",
      status_label: "Duplicate",
      target: target("A-01"),
    },
    {
      index: 4,
      data: { barcode: "OLX-000003", product_sku: "NOPE" },
      status: "invalid",
      status_label: "Invalid",
      target: { organization_name: "Olex Merkez" },
    },
  ],
  errors: [
    {
      index: 3,
      field: "barcode",
      code: "STOCK_IMPORT_DUPLICATE_IN_FILE",
      error: "The barcode appears twice in the file.",
    },
    {
      index: 4,
      field: "product_sku",
      code: "STOCK_IMPORT_PRODUCT_NOT_FOUND",
      error: "Product NOPE was not found.",
    },
  ],
};

const appliedCounts = { applied: 2, duplicate: 1, invalid: 1 };
const undoneCounts = { undone: 1, undo_rejected: 1, duplicate: 1, invalid: 1 };

function job(status: string, extra: Record<string, unknown> = {}) {
  return {
    uuid: JOB,
    resource: "tenant.stock.units",
    format: "csv",
    status,
    mapping: { barcode: "barcode", product_sku: "product_sku" },
    source_filename: "opening-stock.csv",
    created_at: NOW,
    ...extra,
  };
}

function entryLine(n: number, location: boolean) {
  return {
    uuid: `0b9c4c1e-0000-4000-8000-00000000l75${n}`,
    unit_uuid: `0b9c4c1e-0000-4000-8000-00000000u75${n}`,
    barcode: `OLX-00000${n}`,
    unit_kind: "serial",
    unit_status: "in_stock",
    quantity: 1,
    product: PRODUCT,
    location: location
      ? { uuid: "loc-a01", code: "A-01", full_code: "A-01" }
      : null,
    label_url: `/v1/stock/labels/units.pdf?barcode=OLX-00000${n}`,
    entry_movement_uuid: `0b9c4c1e-0000-4000-8000-00000000m75${n}`,
    placement_movement_uuid: null,
    undone: false,
  };
}

/** Stock import endpoints; the apply records the import entry. */
function mockStockImport(api: WarehouseMock) {
  let polls = 0;
  let rolledBack = false;
  api.extra.push(async ({ method, path, url, route, ok }) => {
    if (method === "GET" && path === "/v1/stock/import/sample") {
      expect(url.searchParams.get("format")).toBe("xlsx");
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type":
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
          "Content-Disposition": 'attachment; filename="stock-sample.xlsx"',
        },
        body: Buffer.from("PK-xlsx"),
      });
      return true;
    }
    if (method === "POST" && path === "/v1/stock/import") {
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
    if (method === "GET" && path === JOBS) {
      polls++;
      if (rolledBack) {
        await ok(
          job("rolled_back", {
            preview_summary: { ...previewSummary, counts: undoneCounts },
          }),
        );
      } else if (polls < 2) {
        await ok(job("applying", { preview_summary: previewSummary }));
      } else {
        if (!api.entries.some((e) => e.uuid === ENTRY)) {
          api.entries.unshift({
            uuid: ENTRY,
            mode: "import",
            status: "confirmed",
            note: null,
            warehouse: null,
            import_batch_uuid: JOB,
            import_job_uuid: JOB,
            import_source_filename: "opening-stock.csv",
            import_status: "applied",
            lines: [entryLine(1, true), entryLine(2, false)],
            label_batches: [],
            created_at: NOW,
            confirmed_at: NOW,
            cancelled_at: null,
          });
        }
        await ok(
          job("applied", {
            applied_at: NOW,
            rollback_until: "2099-10-17T09:00:00Z",
            preview_summary: { ...previewSummary, counts: appliedCounts },
          }),
        );
      }
      return true;
    }
    if (method === "POST" && path === `${JOBS}/rollback`) {
      rolledBack = true;
      // OLX-000002 moved after the import: the undo keeps it in stock.
      const entry = api.entries.find((e) => e.uuid === ENTRY);
      if (entry) {
        entry.import_status = "partially_undone";
        (entry.lines as { undone: boolean }[])[0].undone = true;
      }
      await ok(
        job("rolled_back", {
          preview_summary: { ...previewSummary, counts: undoneCounts },
        }),
      );
      return true;
    }
    return false;
  });
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
  // The revision-request widget (TEC-628) floats over the sheet footer.
  await context.route("https://issue.technowide.tr/**", (route) =>
    route.abort(),
  );
});

test("center imports opening stock from Excel, opens the entry, undoes", async ({
  page,
}) => {
  const api = await mockWarehouse(page);
  api.permissions.push("stock.import", "tenant.imports.read");
  mockStockImport(api);

  await page.goto(`${base}/entries`);
  await page.getByTestId("stock-import-open").click();
  const wizard = page.getByTestId("import-wizard");
  await expect(wizard).toBeVisible();
  await expect(
    wizard.getByRole("heading", { name: "Stock import" }),
  ).toBeVisible();

  // Template (Excel) from the stock import sample endpoint.
  const download = page.waitForEvent("download");
  await wizard.getByTestId("import-download-template").click();
  expect((await download).suggestedFilename()).toBe("sample.xlsx");

  // The filled file.
  await wizard.locator("#import-file").setInputFiles({
    name: "opening-stock.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(CSV),
  });
  await expect(wizard.getByTestId("import-detected-format")).toHaveText("CSV");
  await wizard.getByTestId("import-next").click();

  // Columns: barcode / SKU / location matched from the headers.
  const columns = wizard.getByTestId("import-columns");
  await expect(columns.locator('tr[data-field="barcode"]')).toContainText(
    "OLX-000001",
  );
  await wizard.getByTestId("import-next").click();

  // Preview: two new units, one duplicate and one invalid row.
  await expect(wizard.getByTestId("import-status-card-new")).toContainText("2");
  await expect(
    wizard.getByTestId("import-status-card-duplicate"),
  ).toContainText("1");
  await expect(wizard.getByTestId("import-status-card-invalid")).toContainText(
    "1",
  );
  await expect(wizard.getByText("Product NOPE was not found.")).toBeVisible();
  await wizard.getByTestId("import-next").click();

  // Confirm: the stock sentence with the organization and the location.
  await expect(wizard.getByTestId("import-confirm-sentence")).toHaveText(
    "2 units will be added to the Olex Merkez stock and a single stock entry document will be created. 1 of them will be placed on the location given in the file. 2 rows will be skipped.",
  );
  const confirm = wizard.getByTestId("import-confirm");
  await expect(confirm).toHaveText("Import 2 units");
  await wizard.getByTestId("import-ack-skipped").click();
  await confirm.click();

  // Result: applied, link to the recorded import entry.
  const result = wizard.getByTestId("import-result");
  await expect(result).toHaveAttribute("data-status", "applied");
  const entryLink = wizard.getByTestId("import-result-entry");
  await expect(entryLink).toHaveText("Open the stock entry");
  await expect(entryLink).toHaveAttribute("href", `${base}/entries/${ENTRY}`);

  // Undo: one unit moved after the import and stays.
  await wizard.getByTestId("import-undo").click();
  await expect(result).toHaveAttribute("data-status", "rolled_back");
  await expect(wizard.getByTestId("import-result-title")).toHaveText(
    "The import was partially undone",
  );

  // The entry: source file, import job link, partial undo explanation.
  await wizard.getByTestId("import-result-entry").click();
  await expect(page).toHaveURL(new RegExp(`${base}/entries/${ENTRY}$`));
  const source = page.getByTestId("entry-import-source");
  await expect(source).toContainText("opening-stock.csv");
  await expect(page.getByTestId("entry-import-job")).toHaveAttribute(
    "href",
    `/t/${WAREHOUSE_SLUG}/settings/imports/${JOB}`,
  );
  await expect(page.getByTestId("entry-import-partial")).toContainText(
    "Units that moved after the import were not undone and stay in stock (1)",
  );

  // The list shows the import entry with its mode.
  await page.goto(`${base}/entries`);
  await expect(page.getByText("Import", { exact: true }).first()).toBeVisible();
  expect(
    api.calls.some((c) =>
      c.startsWith(
        `GET /v1/warehouse/stock-entries?limit=1&offset=0&mode=import&import_job_uuid=${JOB}`,
      ),
    ),
  ).toBe(true);
  expect(
    api.calls.filter((c) => c.startsWith(`POST ${JOBS}/confirm`)),
  ).toHaveLength(1);
});

test("a distributor does not get the Excel import", async ({ page }) => {
  const api = await mockWarehouse(page);
  api.type = "distributor";
  api.permissions.push("stock.import", "tenant.imports.read");

  await page.goto(`${base}/entries`);
  await expect(page.getByTestId("entry-new")).toBeVisible();
  await expect(page.getByTestId("stock-import-open")).toHaveCount(0);
});
