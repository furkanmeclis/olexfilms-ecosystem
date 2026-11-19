import { describe, expect, it } from "vitest";

import type { Order } from "@/features/orders/services/orders.service";

import {
  addLine,
  boxConversion,
  boxesOf,
  convertToBoxes,
  linesBelowAssigned,
  linesFromOrder,
  lineKey,
  productMeasures,
  staleMeterLine,
  stepMeters,
  toItemInputs,
  validateOrderForm,
  wholeBoxBetter,
  type OrderFormLine,
  type PickedProduct,
} from "./form";

const FILM: PickedProduct = {
  uuid: "f1",
  sku: "BLK",
  name: "Black PPF",
  unit_type: "piece",
  sellable_by_meter: true,
  box_meters: 15,
  min_cut: 2,
  usage_credits: 5,
};

const meterLine = (amount: string, over: Partial<OrderFormLine> = {}) => ({
  ...addLine([], FILM, "meter")[0]!,
  amount,
  ...over,
});

describe("TEC-1239 box and meter lines", () => {
  it("offers the measures of a product", () => {
    expect(productMeasures(FILM)).toEqual(["box", "meter"]);
    expect(productMeasures({ unit_type: "piece" })).toEqual(["box"]);
    expect(productMeasures({ unit_type: "roll_meter" })).toEqual(["meter"]);
  });

  it("keeps one line per product and measure (K10)", () => {
    let lines = addLine([], FILM, "box");
    lines = addLine(lines, FILM, "meter");
    lines = addLine(lines, FILM, "meter");
    expect(lines.map(lineKey)).toEqual(["f1:box", "f1:meter"]);
    expect(lines[0]!.amount).toBe("1");
    expect(lines[1]!.amount).toBe("");
    // K3: the cut line starts with the product's credits.
    expect(lines[1]!.line_credits).toBe("5");
    expect(lines[0]!.line_credits).toBeUndefined();
  });

  it("validates the meters, the minimum cut and the credits (K6)", () => {
    expect(validateOrderForm([meterLine("0")]).lines).toEqual({
      "f1:meter": "meters_invalid",
    });
    expect(validateOrderForm([meterLine("1.5")]).lines).toEqual({
      "f1:meter": "below_min_cut",
    });
    expect(validateOrderForm([meterLine("2")]).lines).toEqual({});
    expect(
      validateOrderForm([meterLine("3", { line_credits: "0" })]).lines,
    ).toEqual({ "f1:meter": "line_credits_invalid" });
    expect(
      validateOrderForm([meterLine("3", { line_credits: "1001" })]).lines,
    ).toEqual({ "f1:meter": "line_credits_invalid" });
    // A roll_meter line has no minimum cut check here and no credits.
    const roll = addLine([], { ...FILM, unit_type: "roll_meter" })[0]!;
    expect(validateOrderForm([{ ...roll, amount: "0.5" }]).lines).toEqual({});
  });

  it("sends 1 box + 3 m with the measure and the credits", () => {
    const lines = [
      ...addLine([], FILM, "box"),
      meterLine("3,0", { line_credits: "7" }),
    ];
    expect(toItemInputs(lines)).toEqual([
      { product_uuid: "f1", quantity: 1, sale_measure: "box" },
      {
        product_uuid: "f1",
        meters: "3.0",
        sale_measure: "meter",
        line_credits: 7,
      },
    ]);
    // K12: the plotter mark of a cut line shares the line credits.
    expect(
      toItemInputs([meterLine("3", { line_credits: "7", plotter_cut: true })]),
    ).toEqual([
      {
        product_uuid: "f1",
        meters: "3",
        sale_measure: "meter",
        line_credits: 7,
        plotter_cut: true,
        plotter_credits: 7,
      },
    ]);
  });

  it("steps the meters by 0.5 and keeps at least one step", () => {
    expect(stepMeters("", 1)).toBe("0.5");
    expect(stepMeters("", -1)).toBe("0.5");
    expect(stepMeters("2,3", 1)).toBe("2.5");
    expect(stepMeters("2.3", -1)).toBe("2");
    expect(stepMeters("3", 1)).toBe("3.5");
    expect(stepMeters("3", -1)).toBe("2.5");
    expect(stepMeters("0.5", -1)).toBe("0.5");
  });

  it("shows the boxes and the whole-box warning", () => {
    expect(boxesOf(3, 15)).toBe(0.2);
    expect(boxesOf(3, null)).toBeNull();
    expect(wholeBoxBetter(meterLine("14.99"))).toBe(false);
    expect(wholeBoxBetter(meterLine("15"))).toBe(true);
    expect(boxConversion(meterLine("15"))).toEqual({ boxes: 1, rest: 0 });
    expect(boxConversion(meterLine("18"))).toEqual({ boxes: 1, rest: 3 });
    // A rest below the minimum cut is dropped.
    expect(boxConversion(meterLine("16"))).toEqual({ boxes: 1, rest: 0 });
    expect(boxConversion(meterLine("31"))).toEqual({ boxes: 2, rest: 0 });
  });

  it("converts a meter line into boxes in one click", () => {
    const only = convertToBoxes([meterLine("15")], "f1:meter");
    expect(only.map((l) => [lineKey(l), l.amount])).toEqual([["f1:box", "1"]]);
    const withBox = convertToBoxes(
      [{ ...addLine([], FILM, "box")[0]!, amount: "2" }, meterLine("33")],
      "f1:meter",
    );
    expect(withBox.map((l) => [lineKey(l), l.amount])).toEqual([
      ["f1:box", "4"],
      ["f1:meter", "3"],
    ]);
  });

  it("loads a saved draft and flags a meter line whose switch was turned off", () => {
    type Item = NonNullable<Order["items"]>[number];
    const item = (over: Partial<Item>): Item =>
      ({
        uuid: "i1",
        product: {
          uuid: "f1",
          sku: "BLK",
          name: "Black PPF",
          unit_type: "piece",
          sellable_by_meter: false,
          box_meters: "15.00",
          min_cut_meters: "2.00",
        },
        quantity: null,
        meters: "3.00",
        sale_measure: "meter",
        line_credits: 6,
        plotter_cut: false,
        plotter_credits: null,
        assigned: "0",
        ...over,
      }) as Item;
    const order = {
      items: [
        item({
          uuid: "i0",
          quantity: 1,
          meters: null,
          sale_measure: "box",
          line_credits: null,
        }),
        item({ assigned: "3" }),
      ],
    } as Order;
    const lines = linesFromOrder(order);
    expect(lines.map(lineKey)).toEqual(["f1:box", "f1:meter"]);
    expect(lines[1]).toMatchObject({
      amount: "3.00",
      line_credits: "6",
      box_meters: 15,
      min_cut: 2,
      sellable_by_meter: false,
    });
    expect(staleMeterLine(lines[1]!)).toBe(true);
    expect(staleMeterLine(lines[0]!)).toBe(false);
    // The assigned meters are checked against the meter line only.
    const edited = lines.map((l) => ({ ...l, amount: "2.5" }));
    expect([...linesBelowAssigned(order, edited)]).toEqual(["f1:meter"]);
  });
});
