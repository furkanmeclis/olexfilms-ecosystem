import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import { chooseOption } from "./support/pickers";
import {
  mockWarehouse,
  PRODUCT,
  WAREHOUSE_SLUG,
  type WarehouseMock,
} from "./support/warehouse-mock";

/**
 * TEC-764: location and label screens against the mocked BFF — bulk
 * generation (room → wizard → preview → confirm → new bins in the tree),
 * the stock sheet of a bin, drag and drop ordering by keyboard and the
 * label template settings (create with preview, default, delete).
 */

const NOW = "2026-10-01T09:00:00Z";
type Json = Record<string, unknown>;
type Node = Json & { uuid: string; code: string; full_code: string };

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

/** WH1 (and WH2) with room R1 holding the aisle A. */
function seed(api: WarehouseMock) {
  const wh = (uuid: string, code: string, sort: number): Node => ({
    uuid,
    code,
    full_code: code,
    name: code,
    address: null,
    active: true,
    require_location_placement: false,
    sort_order: sort,
    created_at: NOW,
    updated_at: NOW,
  });
  api.warehouses.push(
    wh("0b9c4c1e-0000-4000-8000-a00000000001", "WH1", 0),
    wh("0b9c4c1e-0000-4000-8000-a00000000002", "WH2", 1),
  );
  api.rooms.push({
    uuid: "0b9c4c1e-0000-4000-8000-b00000000001",
    warehouse_uuid: api.warehouses[0].uuid,
    code: "R1",
    full_code: "WH1-R1",
    name: "R1",
    active: true,
    sort_order: 0,
    created_at: NOW,
    updated_at: NOW,
  });
  api.locations.push(location(api, "A", "aisle", null));
}

let seq = 0;
function location(
  api: WarehouseMock,
  code: string,
  type: string,
  parent: Node | null,
): Node {
  seq += 1;
  const room = api.rooms[0];
  return {
    uuid: `0b9c4c1e-0000-4000-8000-e${String(seq).padStart(11, "0")}`,
    warehouse_uuid: room.warehouse_uuid,
    room_uuid: room.uuid,
    parent_uuid: parent?.uuid ?? null,
    type,
    code,
    full_code: `${parent ? parent.full_code : room.full_code}-${code}`,
    name: code,
    active: true,
    sort_order: 0,
    created_at: NOW,
    updated_at: NOW,
  };
}

/** POST /v1/warehouse/locations/generate like the backend: reuse or create. */
function generate(api: WarehouseMock, body: Json) {
  const levels = body.levels as Json[];
  const codesOf = (lv: Json): string[] =>
    lv.codes
      ? (lv.codes as string[])
      : Array.from(
          { length: Number(lv.to) - Number(lv.from) + 1 },
          (_, i) =>
            `${lv.prefix ?? ""}${String(Number(lv.from) + i).padStart(
              Number(lv.pad ?? 0),
              "0",
            )}`,
        );
  let frontier: (Node | null)[] = [
    body.parent_uuid
      ? (api.locations.find((l) => l.uuid === body.parent_uuid) ?? null)
      : null,
  ];
  let created = 0;
  let existing = 0;
  for (const lv of levels) {
    const next: Node[] = [];
    for (const parent of frontier) {
      for (const code of codesOf(lv)) {
        let node = api.locations.find(
          (l) => l.parent_uuid === (parent?.uuid ?? null) && l.code === code,
        );
        if (node) existing += 1;
        else {
          node = location(api, code, String(lv.type), parent);
          api.locations.push(node);
          created += 1;
        }
        next.push(node);
      }
    }
    frontier = next;
  }
  return { created, existing };
}

test("locations: bulk generate → preview → confirm → bin stock", async ({
  page,
}) => {
  const api = await mockWarehouse(page);
  seed(api);
  api.extra.push(async ({ method, path, url, body, ok, route }) => {
    if (method === "POST" && path === "/v1/warehouse/locations/generate") {
      await ok(generate(api, body), 201);
      return true;
    }
    if (method === "POST" && path === "/v1/warehouse/warehouses/reorder") {
      const order = body.uuids as string[];
      api.warehouses.sort(
        (a, b) => order.indexOf(a.uuid) - order.indexOf(b.uuid),
      );
      await route.fulfill({ status: 204, body: "" });
      return true;
    }
    if (
      method === "GET" &&
      /^\/v1\/stock\/locations\/[^/]+\/products$/.test(path)
    ) {
      await ok({
        items: [
          {
            product: {
              ...PRODUCT,
              unit_type: "roll_meter",
              uses_fixed_barcode: false,
              active: true,
              category: { uuid: "c1", name: "PPF" },
            },
            quantity: 2,
            meters: "30.00",
            fixed_barcodes: [],
            updated_at: NOW,
          },
        ],
        total: 1,
        limit: 10,
        offset: 0,
      });
      return true;
    }
    if (
      method === "GET" &&
      /^\/v1\/stock\/organizations\/[^/]+\/units$/.test(path)
    ) {
      const bin = api.locations.find(
        (l) => l.uuid === url.searchParams.get("location_uuid"),
      );
      await ok({
        items: ["OLEX-00000001", "OLEX-00000002"].map((barcode, i) => ({
          uuid: `0b9c4c1e-0000-4000-8000-f0000000000${i}`,
          barcode,
          unit_kind: "serial",
          status: "placed",
          quantity: 1,
          initial_meters: "15.00",
          remaining_meters: "15.00",
          product: {
            ...PRODUCT,
            unit_type: "roll_meter",
            uses_fixed_barcode: false,
          },
          location: bin
            ? { uuid: bin.uuid, code: bin.full_code, name: bin.name }
            : null,
          purchase_price: null,
          updated_at: NOW,
        })),
        total: 2,
        limit: 10,
        offset: 0,
      });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${WAREHOUSE_SLUG}/warehouse/locations`);
  await expect(page.getByTestId("location-node")).toHaveCount(1);

  // --- Wizard: structure ---
  await page.getByTestId("location-bulk").click();
  const wizard = page.getByTestId("bulk-wizard");
  await expect(wizard).toHaveAttribute("data-step", "structure");
  const levels = wizard.getByTestId("bulk-level");
  await expect(levels).toHaveCount(3);
  await expect(levels.nth(0)).toHaveAttribute("data-type", "aisle");
  await expect(levels.nth(0).getByTestId("bulk-level-codes")).toHaveValue(
    "A, B",
  );
  // Shelves 01..02 under each aisle; bins stay 01..04.
  await levels.nth(1).getByTestId("bulk-level-to").fill("2");
  await expect(levels.nth(1).getByTestId("bulk-level-count")).toContainText(
    "01, 02",
  );
  // An invalid code blocks the preview until fixed.
  await levels.nth(0).getByTestId("bulk-level-codes").fill("A, B-1");
  await expect(levels.nth(0).getByTestId("bulk-level-error")).toContainText(
    "B-1",
  );
  await expect(wizard.getByTestId("bulk-next")).toBeDisabled();
  await levels.nth(0).getByTestId("bulk-level-codes").fill("A, B");

  // --- Preview ---
  await wizard.getByTestId("bulk-next").click();
  await expect(wizard).toHaveAttribute("data-step", "preview");
  // 2 aisles + 4 shelves + 16 bins; the aisle A already exists.
  await expect(wizard.getByTestId("bulk-preview-total")).toHaveText("22");
  await expect(wizard.getByTestId("bulk-preview-existing")).toHaveText("1");
  await expect(wizard.getByTestId("bulk-preview-new")).toHaveText("21");
  const sample = wizard.getByTestId("bulk-preview-sample");
  await expect(sample).toContainText("WH1-R1-A-01-01");
  await expect(sample).toContainText("WH1-R1-B-02-04");
  await expect(wizard.getByTestId("bulk-large")).toHaveCount(0);

  // --- Confirm ---
  await wizard.getByTestId("bulk-create").click();
  await expect(wizard).toHaveAttribute("data-step", "result");
  await expect(wizard.getByTestId("bulk-result")).toContainText("21");
  expect(api.bodies["POST /v1/warehouse/locations/generate"]).toEqual([
    {
      room_uuid: api.rooms[0].uuid,
      parent_uuid: null,
      levels: [
        { type: "aisle", codes: ["A", "B"] },
        { type: "shelf", from: 1, to: 2, pad: 2, prefix: "" },
        { type: "bin", from: 1, to: 4, pad: 2, prefix: "" },
      ],
    },
  ]);
  await wizard.getByTestId("bulk-close").click();
  await expect(wizard).toHaveCount(0);

  // The new grid is in the tree, expanded.
  await expect(page.getByTestId("location-node")).toHaveCount(22);
  const bin = page.locator('[data-code="WH1-R1-B-02-04"]');
  await expect(bin).toHaveAttribute("aria-level", "3");

  // --- Bin stock sheet ---
  await bin.getByTestId("location-select").first().click();
  const sheet = page.getByTestId("location-stock-sheet");
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText("WH1-R1-B-02-04");
  await expect(sheet.getByTestId("location-stock-product-count")).toHaveText(
    "1",
  );
  await expect(sheet.getByTestId("location-stock-unit-count")).toHaveText("2");
  await expect(sheet.getByTestId("location-stock-product")).toContainText(
    PRODUCT.name,
  );
  await sheet.getByTestId("location-stock-tab-units").click();
  const unit = sheet.getByTestId("location-stock-unit").first();
  await expect(unit).toHaveText("OLEX-00000001");
  await expect(unit).toHaveAttribute(
    "href",
    `/t/${WAREHOUSE_SLUG}/stock/units/OLEX-00000001`,
  );
  const binUuid = await bin.getAttribute("data-uuid");
  expect(
    api.calls.some(
      (c) =>
        c.startsWith("GET /v1/stock/organizations/") &&
        c.includes(`location_uuid=${binUuid}`),
    ),
  ).toBe(true);
  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);

  // --- Drag and drop by keyboard: WH2 above WH1 ---
  const [wh1, wh2] = api.warehouses.map((w) => w.uuid);
  const handle = page
    .getByTestId("warehouse-row")
    .filter({ hasText: "WH1" })
    .getByTestId("sort-handle");
  await handle.focus();
  // dnd-kit measures the list between the keys.
  await page.keyboard.press("Space");
  await page.waitForTimeout(300);
  await page.keyboard.press("ArrowDown");
  await page.waitForTimeout(300);
  await page.keyboard.press("Space");
  await expect
    .poll(() => api.bodies["POST /v1/warehouse/warehouses/reorder"])
    .toEqual([{ uuids: [wh2, wh1] }]);
  await expect(page.getByTestId("warehouse-row").first()).toHaveAttribute(
    "data-code",
    "WH2",
  );
});

test("label templates: create with preview → default → delete", async ({
  page,
}) => {
  const api = await mockWarehouse(page);
  const templates: Json[] = [];
  api.extra.push(async ({ method, path, body, ok, route }) => {
    if (path === "/v1/stock/label-templates") {
      if (method === "GET") {
        await ok({ items: templates });
        return true;
      }
      const tpl = {
        uuid: `0b9c4c1e-0000-4000-8000-d0000000000${templates.length + 1}`,
        logo_text: null,
        logo_image: null,
        created_at: NOW,
        updated_at: NOW,
        ...body,
        width_mm: Number(body.width_mm).toFixed(1),
        height_mm: Number(body.height_mm).toFixed(1),
      };
      templates.push(tpl);
      await ok(tpl, 201);
      return true;
    }
    const one = path.match(/^\/v1\/stock\/label-templates\/([^/]+)$/);
    if (one) {
      const i = templates.findIndex((x) => x.uuid === one[1]);
      if (method === "DELETE") {
        templates.splice(i, 1);
        await route.fulfill({ status: 204, body: "" });
        return true;
      }
      if (method === "PUT") {
        if (body.is_default) {
          for (const x of templates) x.is_default = false;
        }
        templates[i] = {
          ...templates[i],
          ...body,
          kind: templates[i].kind,
          width_mm: Number(body.width_mm).toFixed(1),
          height_mm: Number(body.height_mm).toFixed(1),
        };
        await ok(templates[i]);
        return true;
      }
    }
    return false;
  });

  await page.goto(`/t/${WAREHOUSE_SLUG}/stock/label-templates`);
  await expect(
    page.getByRole("heading", { name: "Label templates" }),
  ).toBeVisible();
  await expect(page.getByText("No label templates")).toBeVisible();

  // --- Create a location template; the preview follows the fields ---
  await page.getByTestId("label-template-new").click();
  const form = page.getByTestId("label-template-form");
  const preview = form.getByTestId("label-preview");
  await expect(preview.getByTestId("label-preview-item")).toHaveCount(3);
  await expect(
    preview.getByTestId("label-preview-item").first(),
  ).toHaveAttribute("data-symbology", "code128");
  await chooseOption(form.getByTestId("label-template-kind"), "location");
  await expect(form.getByTestId("label-template-symbology")).toBeDisabled();
  await expect(form.getByTestId("label-template-columns")).toHaveValue("4");
  await expect(preview.getByTestId("label-preview-item")).toHaveCount(4);
  await expect(
    preview.getByTestId("label-preview-item").first(),
  ).toHaveAttribute("data-symbology", "qr");
  await form.getByTestId("label-template-columns").fill("2");
  await expect(preview.getByTestId("label-preview-item")).toHaveCount(2);
  await chooseOption(form.getByTestId("label-template-logo-mode"), "text");
  await form.getByTestId("label-template-logo-text").fill("OLEX");
  await expect(preview.getByTestId("label-preview-logo").first()).toHaveText(
    "OLEX",
  );

  // Client-side validation before the request.
  await form.getByTestId("label-template-width").fill("5");
  await form.getByTestId("label-template-save").click();
  await expect(form.getByText("Enter 20–200 mm")).toBeVisible();
  expect(api.bodies["POST /v1/stock/label-templates"]).toBeUndefined();
  await form.getByTestId("label-template-width").fill("60");
  await form.getByTestId("label-template-name").fill("Bin QR");
  await form.getByTestId("label-template-save").click();
  await expect(form).toHaveCount(0);
  expect(api.bodies["POST /v1/stock/label-templates"]).toEqual([
    expect.objectContaining({
      name: "Bin QR",
      kind: "location",
      symbology: "qr",
      logo_mode: "text",
      logo_text: "OLEX",
      width_mm: "60",
      height_mm: "50",
      columns: 2,
      is_default: false,
    }),
  ]);
  const row = page.getByTestId("label-template-row");
  await expect(row).toHaveAttribute("data-name", "Bin QR");
  await expect(page.getByTestId("label-template-default")).toHaveCount(0);

  // --- Make it the default ---
  await page.getByRole("button", { name: "Make default" }).click();
  await expect(page.getByTestId("label-template-default")).toBeVisible();
  expect(
    api.bodies[`PUT /v1/stock/label-templates/${String(templates[0].uuid)}`],
  ).toEqual([expect.objectContaining({ is_default: true, name: "Bin QR" })]);

  // --- Delete after confirmation ---
  const uuid = String(templates[0].uuid);
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  const confirm = page.getByRole("dialog");
  await expect(confirm).toContainText("Delete the template Bin QR?");
  await confirm.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page.getByText("No label templates")).toBeVisible();
  expect(api.bodies[`DELETE /v1/stock/label-templates/${uuid}`]).toHaveLength(
    1,
  );
});
