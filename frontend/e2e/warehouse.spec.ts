import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import { mockWarehouse, WAREHOUSE_SLUG } from "./support/warehouse-mock";

/**
 * TEC-231: the warehouse slice 1 against a mocked BFF — build a location
 * tree (warehouse › room › aisle › shelf › bin), resolve the bin's QR on
 * the scan page, then take a printed label into stock: open an entry,
 * scan the unit, scan the bin QR to place it and confirm.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("warehouse: create a location, then enter stock on it", async ({
  page,
}) => {
  const api = await mockWarehouse(page);
  const base = `/t/${WAREHOUSE_SLUG}/warehouse`;

  // --- Location tree ---
  await page.goto(`${base}/locations`);
  await expect(page.getByRole("heading", { name: "Locations" })).toBeVisible();
  await expect(page.getByTestId("warehouses-empty")).toBeVisible();

  const form = page.getByTestId("node-form");
  const save = async (code: string) => {
    await form.getByTestId("node-code").fill(code);
    await form.getByTestId("node-save").click();
    await expect(form).toHaveCount(0);
  };

  await page.getByTestId("warehouse-new").click();
  // Client-side validation: the hyphen is not a code character.
  await form.getByTestId("node-code").fill("WH-1");
  await form.getByTestId("node-save").click();
  await expect(form.getByText("Use letters, digits and _ only")).toBeVisible();
  expect(api.bodies["POST /v1/warehouse/warehouses"]).toBeUndefined();
  await save("wh1");
  await expect(page.getByTestId("warehouse-row")).toHaveAttribute(
    "data-code",
    "WH1",
  );

  await page.getByTestId("room-new").click();
  await save("R1");
  await expect(page.getByTestId("room-row")).toHaveAttribute("data-code", "R1");

  await page.getByTestId("location-new-root").click();
  await expect(form.getByTestId("node-type")).toHaveValue("aisle");
  await save("A");
  const nodes = page.getByTestId("location-node");
  await expect(nodes).toHaveCount(1);

  await page.getByRole("button", { name: "Add under A" }).click();
  // Under an aisle only a shelf is allowed.
  await expect(form.getByTestId("node-type").locator("option")).toHaveText([
    "Shelf",
  ]);
  await save("S1");
  await page.getByRole("button", { name: "Add under S1" }).click();
  await expect(form.getByTestId("node-type")).toHaveValue("bin");
  await save("01");

  await expect(nodes).toHaveCount(3);
  await expect(page.locator('[data-code="WH1-R1-A-S1-01"]')).toHaveAttribute(
    "aria-level",
    "3",
  );
  expect(api.bodies["POST /v1/warehouse/locations"]).toEqual([
    expect.objectContaining({ type: "aisle", code: "A", parent_uuid: null }),
    expect.objectContaining({ type: "shelf", code: "S1" }),
    expect.objectContaining({ type: "bin", code: "01" }),
  ]);

  // --- Universal scan resolves the new bin ---
  await page.goto(`${base}/scan`);
  await page.getByTestId("scan-input").fill("ofw:loc:WH1-R1-A-S1-01");
  await page.getByTestId("scan-input").press("Enter");
  await expect(page.getByTestId("scan-result")).toHaveAttribute(
    "data-type",
    "location",
  );
  await expect(page.getByTestId("scan-result")).toContainText("WH1-R1-A-S1-01");

  // --- Stock entry ---
  await page.goto(`${base}/entries`);
  await expect(page.getByTestId("entries-empty")).toBeVisible();
  await page.getByTestId("entry-new").click();
  await page
    .getByTestId("entry-warehouse")
    .selectOption({ label: "WH1 · WH1" });
  await page.getByTestId("entry-mode").selectOption("with_existing");
  await page.getByTestId("entry-note").fill("Container 12");
  await page.getByTestId("entry-create").click();
  await expect(page).toHaveURL(new RegExp(`${base}/entries/[0-9a-f-]+$`));
  await expect(page.getByTestId("entry-status")).toHaveAttribute(
    "data-status",
    "draft",
  );
  await expect(page.getByTestId("entry-confirm")).toBeDisabled();

  // Unknown label: the server's code is shown, nothing is added.
  await page.getByTestId("entry-barcode-input").fill("OLEX-99999999");
  await page.getByTestId("entry-barcode-input").press("Enter");
  await expect(page.getByTestId("entry-line-error")).toContainText(
    "not a printed label",
  );

  await page.getByTestId("entry-barcode-input").fill("OFW:UNIT:OLEX-00000001");
  await page.getByTestId("entry-barcode-input").press("Enter");
  const line = page.getByTestId("entry-line");
  await expect(line).toHaveCount(1);
  await expect(line).toHaveAttribute("data-barcode", "OLEX-00000001");
  await expect(line.getByTestId("entry-line-location")).toContainText(
    "Not placed",
  );

  await page.getByTestId("entry-location-input").fill("OFW:LOC:WH1-R1-A-S1-01");
  await page.getByTestId("entry-location-input").press("Enter");
  await expect(line.getByTestId("entry-line-location")).toHaveText(
    "WH1-R1-A-S1-01",
  );
  await expect(page.getByTestId("entry-counts")).toHaveText("1 / 1 placed");

  await page.getByTestId("entry-confirm").click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm" })
    .click();
  await expect(page.getByTestId("entry-status")).toHaveAttribute(
    "data-status",
    "confirmed",
  );
  await expect(page.getByTestId("entry-barcode-input")).toHaveCount(0);

  const entryUuid = page.url().split("/").pop();
  expect(api.bodies["POST /v1/warehouse/stock-entries"]).toEqual([
    expect.objectContaining({ mode: "with_existing", note: "Container 12" }),
  ]);
  expect(
    api.bodies[`POST /v1/warehouse/stock-entries/${entryUuid}/place`],
  ).toEqual([
    expect.objectContaining({ location_code: "OFW:LOC:WH1-R1-A-S1-01" }),
  ]);
  expect(api.calls).toContain(
    `POST /v1/warehouse/stock-entries/${entryUuid}/confirm`,
  );
  expect(api.unknown.filter((c) => c.includes("/warehouse"))).toEqual([]);
});
