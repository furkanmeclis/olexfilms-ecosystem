/**
 * TEC-1240 (Kutu/metraj 3/7): the mobile cart and editor order a product by
 * the box and by the meter (K10), check the minimum cut (K6), carry the meter
 * line credits (K3/K12) and convert a meter line to whole boxes in one tap.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addEditLine,
  addLine,
  boxEquivalent,
  type CartLine,
  type CartProduct,
  cartIsValid,
  convertToBoxes,
  editChanged,
  lineError,
  lineKey,
  lineMeasure,
  linesFromItems,
  otherMeasure,
  productOf,
  stepMeters,
  submitPreview,
  toItemInputs,
  validateCart,
  wholeBoxSuggestion,
  withProductInfo,
} from './order-lines.ts';
import { linePrice, type Quote, quoteTotals } from './order-quote.ts';
import { showsMeasure, stepParts } from './order-summary.ts';

/** Black PPF: a box of 15 m, cut from 2 m, sold by the meter, 5 credits a piece. */
const ppf: CartProduct = {
  uuid: 'ppf',
  sku: 'PPF-B',
  name: 'Black PPF',
  unit_type: 'piece',
  roll: { default_meters: 15, min_cut_meters: 2, sellable_by_meter: true },
  usage_credits: 5,
};
const plain: CartProduct = { uuid: 'kit', sku: 'KIT', name: 'Kit', unit_type: 'piece', roll: null };
const roll: CartProduct = { uuid: 'roll', sku: 'R', name: 'Roll', unit_type: 'roll_meter' };

const meterLine = (amount: string, over: Partial<CartLine> = {}): CartLine => ({
  ...addLine([], ppf, 'meter')[0]!,
  amount,
  ...over,
});

test('cart key is product + measure: one box and one meter line of the same product (K10)', () => {
  let lines = addLine([], ppf, 'box');
  lines = addLine(lines, ppf, 'meter');
  assert.equal(lines.length, 2);
  assert.deepEqual(lines.map(lineKey), ['ppf:box', 'ppf:meter']);
  // The same measure again is ignored: one line per product and measure.
  assert.equal(addLine(lines, ppf, 'meter'), lines);
  assert.equal(addLine(lines, ppf, 'box'), lines);
  // The box starts at 1, the meter line at the minimum cut with the product's credits as default.
  assert.equal(lines[0]!.amount, '1');
  assert.equal(lines[1]!.amount, '2');
  assert.equal(lines[1]!.default_credits, 5);
  assert.equal(lines[1]!.line_credits, '');
});

test('a measure the product is not sold by is refused', () => {
  assert.deepEqual(addLine([], plain, 'meter'), []);
  assert.deepEqual(addLine([], roll, 'box'), []);
  assert.equal(addLine([], roll)[0]!.measure, undefined);
  assert.equal(lineMeasure(addLine([], roll)[0]!), 'meter');
});

test('lines saved before TEC-1240 keep their measure from the unit type', () => {
  const old: CartLine = { product_uuid: 'r', sku: 's', name: 'n', unit_type: 'roll_meter', amount: '3' };
  const box: CartLine = { product_uuid: 'p', sku: 's', name: 'n', unit_type: 'piece', amount: '3' };
  assert.equal(lineKey(old), 'r:meter');
  assert.equal(lineKey(box), 'p:box');
});

test('validation errors are keyed by line, so the box and the meter line fail apart', () => {
  const lines = [...addLine([], ppf, 'box'), meterLine('1')];
  const errors = validateCart(lines);
  assert.deepEqual(errors.lines, { 'ppf:meter': 'below_min_cut' });
  assert.equal(cartIsValid(errors), false);
});

test('minimum cut: below it is refused, at it allowed, unchanged stored meters not checked again (K6)', () => {
  assert.equal(lineError(meterLine('1,5')), 'below_min_cut');
  assert.equal(lineError(meterLine('2')), null);
  assert.equal(lineError(meterLine('2,25')), null);
  assert.equal(lineError(meterLine('0')), 'meters_invalid');
  assert.equal(lineError(meterLine('')), 'meters_invalid');
  // A line stored before the minimum was raised keeps its meters; changing them checks again.
  assert.equal(lineError(meterLine('1.50', { kept_meters: '1.50' })), null);
  assert.equal(lineError(meterLine('1.75', { kept_meters: '1.50' })), 'below_min_cut');
});

test('line credits: 1..1000 or empty (the default) (K3)', () => {
  assert.equal(lineError(meterLine('3', { line_credits: '' })), null);
  assert.equal(lineError(meterLine('3', { line_credits: '7' })), null);
  assert.equal(lineError(meterLine('3', { line_credits: '0' })), 'line_credits_invalid');
  assert.equal(lineError(meterLine('3', { line_credits: '1001' })), 'line_credits_invalid');
  assert.equal(lineError(meterLine('3', { line_credits: '2.5' })), 'line_credits_invalid');
});

test('1 box + 3 m: the request sends a box line and a meter line with its credits', () => {
  let lines = addLine([], ppf, 'box');
  lines = addLine(lines, ppf, 'meter').map((l) => (lineMeasure(l) === 'meter' ? { ...l, amount: '3', line_credits: '6' } : l));
  assert.equal(cartIsValid(validateCart(lines)), true);
  assert.deepEqual(toItemInputs(lines), [
    { product_uuid: 'ppf', quantity: 1 },
    { product_uuid: 'ppf', meters: '3', sale_measure: 'meter', line_credits: 6 },
  ]);
  // Empty credits: the server applies the product default (absent field).
  assert.deepEqual(toItemInputs([{ ...lines[1]!, line_credits: '' }]), [{ product_uuid: 'ppf', meters: '3', sale_measure: 'meter' }]);
  // A roll_meter line stays as before (the server derives its measure).
  assert.deepEqual(toItemInputs([{ ...addLine([], roll)[0]!, amount: '4' }]), [{ product_uuid: 'roll', meters: '4' }]);
});

test('K12: a plotter cut meter line shares the line credits and sends no plotter credits of its own', () => {
  const l = meterLine('3', { plotter_cut: true, plotter_credits: '9', line_credits: '4' });
  assert.equal(lineError(l), null);
  assert.deepEqual(toItemInputs([l]), [{ product_uuid: 'ppf', meters: '3', plotter_cut: true, sale_measure: 'meter', line_credits: 4 }]);
});

test('submit preview lists the meter line in meters and the box line in pieces', () => {
  const lines = [...addLine([], ppf, 'box'), meterLine('3')];
  assert.deepEqual(
    submitPreview(lines).rows.map((r) => r.roll),
    [false, true],
  );
});

test('stepMeters: ±0.5 m, never below the minimum cut, keeps the decimal comma', () => {
  assert.equal(stepMeters('3', 1, 2), '3.5');
  assert.equal(stepMeters('3,5', -1, 2), '3');
  assert.equal(stepMeters('2,5', -1, 2), '2');
  assert.equal(stepMeters('2', -1, 2), '2');
  assert.equal(stepMeters('', 1, 2), '2');
  assert.equal(stepMeters('x', -1, null), '0.5');
  assert.equal(stepMeters('0.5', -1, null), '0.5');
  assert.equal(stepMeters('3,25', 1, 2), '3,75');
});

test('box equivalent: meters / box length, one decimal; none on box lines or without a length', () => {
  assert.equal(boxEquivalent(meterLine('3')), 0.2);
  assert.equal(boxEquivalent(meterLine('22,5')), 1.5);
  assert.equal(boxEquivalent({ ...meterLine('3'), box_meters: null }), null);
  assert.equal(boxEquivalent(addLine([], ppf, 'box')[0]!), null);
});

test('whole box suggestion: a remnant-sized rest suggests a box; a box plus a valid rest splits', () => {
  // 14 of 15 m would leave 1 m (< min cut 2): a whole box fits better.
  assert.deepEqual(wholeBoxSuggestion(meterLine('14')), { boxes: 1, meters: 0 });
  assert.deepEqual(wholeBoxSuggestion(meterLine('15')), { boxes: 1, meters: 0 });
  assert.deepEqual(wholeBoxSuggestion(meterLine('29')), { boxes: 2, meters: 0 });
  // 18 m = 1 box + 3 m (3 ≥ min cut).
  assert.deepEqual(wholeBoxSuggestion(meterLine('18')), { boxes: 1, meters: 3 });
  // A short cut, or no box length: no suggestion.
  assert.equal(wholeBoxSuggestion(meterLine('3')), null);
  assert.equal(wholeBoxSuggestion(meterLine('12')), null);
  assert.equal(wholeBoxSuggestion({ ...meterLine('14'), box_meters: null }), null);
  // Without a minimum cut, 10 % of the box is the threshold.
  assert.deepEqual(wholeBoxSuggestion(meterLine('13,6', { min_cut: null })), { boxes: 1, meters: 0 });
  assert.equal(wholeBoxSuggestion(meterLine('13', { min_cut: null })), null);
});

test('convertToBoxes: one tap moves the boxes to the box line (added or increased), the rest stays', () => {
  const only = [meterLine('14')];
  const a = convertToBoxes(only, 'ppf:meter');
  assert.deepEqual(
    a.map((l) => [lineKey(l), l.amount]),
    [['ppf:box', '1']],
  );
  const both = [...addLine([], ppf, 'box').map((l) => ({ ...l, amount: '2' })), meterLine('18')];
  const b = convertToBoxes(both, 'ppf:meter');
  assert.deepEqual(
    b.map((l) => [lineKey(l), l.amount]),
    [
      ['ppf:box', '3'],
      ['ppf:meter', '3'],
    ],
  );
  // Nothing to suggest: the lines come back unchanged.
  const short = [meterLine('3')];
  assert.equal(convertToBoxes(short, 'ppf:meter'), short);
  // The editor's new box line gets its extra fields.
  const edited = convertToBoxes([{ ...meterLine('15'), assigned: 0 }], 'ppf:meter', { assigned: 0, plotter_cut: false });
  assert.equal(edited[0]!.assigned, 0);
  assert.equal(edited[0]!.plotter_cut, false);
  assert.equal(lineMeasure(edited[0]!), 'box');
});

test('otherMeasure / productOf: "+ Meter" under a box line until the meter line exists', () => {
  const box = addLine([], ppf, 'box');
  assert.equal(otherMeasure(box, box[0]!), 'meter');
  const both = addLine(box, productOf(box[0]!), 'meter');
  assert.equal(both.length, 2);
  assert.equal(both[1]!.min_cut, 2);
  assert.equal(both[1]!.box_meters, 15);
  assert.equal(otherMeasure(both, both[0]!), null);
  assert.equal(otherMeasure(addLine([], plain), addLine([], plain)[0]!), null);
});

test('edit: the order lines keep their measure, meters and credits; the same model validates them', () => {
  const lines = linesFromItems(
    [
      { product: ppf, quantity: 1, meters: null, assigned: '0', plotter_cut: false, sale_measure: 'box', line_credits: null },
      { product: ppf, quantity: null, meters: '3.00', assigned: '0', plotter_cut: false, sale_measure: 'meter', line_credits: 5 },
    ],
    false,
  );
  assert.deepEqual(lines.map(lineKey), ['ppf:box', 'ppf:meter']);
  assert.equal(lines[1]!.line_credits, '5');
  assert.equal(lines[1]!.kept_meters, '3.00');
  // Catalog settings fill in once read; the stored 3 m stays valid even with a higher minimum.
  const info = new Map([[ppf.uuid, { ...ppf, roll: { ...ppf.roll!, min_cut_meters: 4 } }]]);
  const enriched = withProductInfo(lines, info);
  assert.equal(enriched[1]!.min_cut, 4);
  assert.equal(enriched[1]!.default_credits, 5);
  assert.equal(lineError(enriched[1]!), null);
  assert.equal(lineError({ ...enriched[1]!, amount: '3.5' }), 'below_min_cut');
  assert.equal(withProductInfo(enriched, info), enriched);
  // 1 box + 3 m edited to 1 box + 4 m with 7 credits.
  const next = enriched.map((l) => (lineMeasure(l) === 'meter' ? { ...l, amount: '4', line_credits: '7' } : l));
  assert.equal(editChanged(lines, next), true);
  assert.equal(editChanged(lines, enriched), false);
  assert.deepEqual(toItemInputs(next), [
    { product_uuid: 'ppf', quantity: 1 },
    { product_uuid: 'ppf', meters: '4', sale_measure: 'meter', line_credits: 7 },
  ]);
  // Credits alone count as a change.
  assert.equal(editChanged(lines, enriched.map((l) => (lineMeasure(l) === 'meter' ? { ...l, line_credits: '8' } : l))), true);
});

test('edit: a meter line is added next to the box line of the same product', () => {
  const lines = linesFromItems([{ product: ppf, quantity: 1, meters: null, assigned: '1', plotter_cut: true, sale_measure: 'box' }], true);
  const added = addEditLine(lines, ppf, true, 'meter');
  assert.deepEqual(added.map(lineKey), ['ppf:box', 'ppf:meter']);
  assert.equal(added[1]!.assigned, 0);
  assert.equal(added[1]!.plotter_cut, false);
  assert.equal(addEditLine(added, ppf, true, 'meter'), added);
});

test('quote: a meter line is priced by the meter price (K2), the box line by the box price', () => {
  const quote: Quote = {
    currency: 'USD',
    first_order: false,
    discount_pct: '10',
    try_rate: null,
    items: [
      {
        product_uuid: 'ppf',
        list_unit_price: '400',
        unit_price: '360',
        price_source: 'list',
        sellable_by_meter: true,
        meter_list_unit_price: '30',
        meter_unit_price: '27',
      },
    ],
  };
  const box = addLine([], ppf, 'box')[0]!;
  const meter = meterLine('3');
  assert.equal(linePrice(box, quote).total, 360);
  assert.equal(linePrice(meter, quote).unit, 27);
  assert.equal(linePrice(meter, quote).total, 81);
  assert.equal(linePrice(meterLine('2,5'), quote).total, 67.5);
  // A manual discount applies to the meter list price.
  assert.equal(linePrice({ ...meter, discount_pct: '20' }, quote).unit, 24);
  assert.equal(quoteTotals([box, meter], quote).total, 441);
});

test('order detail: the measure badge on a meter line and on the box line of the same product', () => {
  const box = { product: { uuid: 'ppf', unit_type: 'piece' }, sale_measure: 'box' as const };
  const meter = { product: { uuid: 'ppf', unit_type: 'piece' }, sale_measure: 'meter' as const };
  const other = { product: { uuid: 'kit', unit_type: 'piece' }, sale_measure: 'box' as const };
  const rollLine = { product: { uuid: 'roll', unit_type: 'roll_meter' }, sale_measure: 'meter' as const };
  const items = [box, meter, other, rollLine];
  assert.equal(showsMeasure(box, items), true);
  assert.equal(showsMeasure(meter, items), true);
  assert.equal(showsMeasure(other, items), false);
  assert.equal(showsMeasure(rollLine, items), false);
  assert.equal(showsMeasure(box, [box]), false);
});

test('timeline: a meter line change names its measure', () => {
  const labels = { box: 'Kutu', meter: 'Metre' };
  const parts = stepParts(
    {
      action: 'items_edited',
      changes: [
        { product: 'Black PPF', before: '1', after: '2', sale_measure: 'box' },
        { product: 'Black PPF', before: '3.00', after: '4.00', sale_measure: 'meter' },
        { product: 'Kit', before: null, after: '1', sale_measure: 'box' },
      ],
    },
    labels,
  );
  assert.deepEqual(
    parts.map((p) => p.params?.product),
    ['Black PPF · Kutu', 'Black PPF · Metre', 'Kit'],
  );
});
