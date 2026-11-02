import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  canOpenStep,
  customerStepError,
  finishBlocker,
  finishChoices,
  initialParts,
  leaveBlocker,
  nextStep,
  parseKm,
  parseParts,
  partsForProduct,
  partsFromItems,
  previousStep,
  resolveStep,
  resumeStep,
  servicePatch,
  stepBlocker,
  summaryError,
  summaryFromService,
  wizardSteps,
} from './wizard.ts';

const base = {
  km: 1200,
  notes: null,
  service_no: 'DS12AB34CD',
  has_measurement: false,
  vin: 'WBA00000000000001',
  service_no_editable: true,
};

describe('wizard steps', () => {
  it('follows the legacy order and adds photos only with the photo standard', () => {
    assert.deepEqual(wizardSteps(false), ['customer', 'parts', 'products', 'summary']);
    assert.deepEqual(wizardSteps(true), ['customer', 'photos', 'parts', 'products', 'summary']);
  });

  it('moves forward and back inside the visible steps', () => {
    const steps = wizardSteps(false);
    assert.equal(nextStep('customer', steps), 'parts');
    assert.equal(nextStep('summary', steps), null);
    assert.equal(previousStep('parts', steps), 'customer');
    assert.equal(previousStep('customer', steps), null);
    assert.equal(resumeStep(steps), 'parts');
    assert.equal(resumeStep(wizardSteps(true)), 'photos');
  });

  it('keeps the later steps closed until the draft exists and the photos are done', () => {
    const steps = wizardSteps(true);
    assert.ok(canOpenStep('customer', { hasService: false, steps }));
    assert.ok(!canOpenStep('parts', { hasService: false, steps }));
    assert.ok(canOpenStep('photos', { hasService: true, photosMissing: true, steps }));
    assert.ok(!canOpenStep('parts', { hasService: true, photosMissing: true, steps }));
    assert.ok(canOpenStep('summary', { hasService: true, photosMissing: false, steps }));
  });

  it('resolves hidden and locked steps', () => {
    const ok = { hasService: true, partsCount: 1, itemCount: 1 };
    assert.equal(resolveStep('summary', { hasService: false, steps: wizardSteps(false) }), 'customer');
    assert.equal(resolveStep('photos', { ...ok, steps: wizardSteps(false) }), 'parts');
    assert.equal(resolveStep('products', { ...ok, photosMissing: true, steps: wizardSteps(true) }), 'photos');
    assert.equal(resolveStep('products', { ...ok, steps: wizardSteps(true) }), 'products');
  });
});

describe('required parts and products (legacy rule, every role)', () => {
  const steps = wizardSteps(false);

  it('needs a body part before products and a product before the summary', () => {
    const none = { hasService: true, partsCount: 0, itemCount: 0, steps };
    assert.equal(leaveBlocker('parts', none), 'parts');
    assert.equal(stepBlocker('products', none), 'parts');
    assert.equal(stepBlocker('summary', none), 'parts');
    const parts = { ...none, partsCount: 2 };
    assert.equal(leaveBlocker('parts', parts), null);
    assert.equal(leaveBlocker('products', parts), 'items');
    assert.equal(stepBlocker('summary', parts), 'items');
    assert.equal(stepBlocker('summary', { ...parts, itemCount: 1 }), null);
  });

  it('keeps the progress bar from skipping a gate', () => {
    const ctx = { hasService: true, partsCount: 0, itemCount: 3, steps };
    assert.ok(canOpenStep('parts', ctx));
    assert.ok(!canOpenStep('products', ctx));
    assert.ok(!canOpenStep('summary', ctx));
    assert.ok(!canOpenStep('summary', { ...ctx, hasService: false }));
    assert.equal(stepBlocker('summary', { ...ctx, hasService: false }), 'service');
  });

  it('checks the photos first, then parts, then products', () => {
    const ctx = { hasService: true, photosMissing: true, partsCount: 0, itemCount: 0, steps: wizardSteps(true) };
    assert.equal(stepBlocker('summary', ctx), 'photos');
    assert.equal(stepBlocker('summary', { ...ctx, photosMissing: false }), 'parts');
  });

  it('sends the summary back to products when the last product is removed', () => {
    const ctx = { hasService: true, partsCount: 1, steps };
    assert.equal(resolveStep('summary', { ...ctx, itemCount: 1 }), 'summary');
    assert.equal(resolveStep('summary', { ...ctx, itemCount: 0 }), 'products');
    assert.equal(resolveStep('summary', { ...ctx, partsCount: 0, itemCount: 1 }), 'parts');
  });

  it('starts a resumed draft from its items when this device has no selection', () => {
    const items = [{ applied_parts: ['body_kaput'] }, { applied_parts: ['body_kaput', 'body_tavan'] }];
    assert.deepEqual(initialParts(null, items), ['body_kaput', 'body_tavan']);
    assert.deepEqual(initialParts([], items), ['body_kaput', 'body_tavan']);
    assert.deepEqual(initialParts(['window_on_cam'], items), ['window_on_cam']);
    assert.deepEqual(initialParts(null, []), []);
  });
});

describe('first step', () => {
  it('needs a customer, a vehicle with a model and a valid km', () => {
    assert.equal(customerStepError({ customer: null, vehicle: null, km: '' }), 'customer');
    assert.equal(customerStepError({ customer: {}, vehicle: null, km: '' }), 'vehicle');
    assert.equal(customerStepError({ customer: {}, vehicle: { car_model: null }, km: '' }), 'vehicle');
    assert.equal(customerStepError({ customer: {}, vehicle: { car_model: {} }, km: '12a' }), 'km');
    assert.equal(customerStepError({ customer: {}, vehicle: { car_model: {} }, km: '' }), null);
  });

  it('parses km with grouping and bounds', () => {
    assert.equal(parseKm(''), null);
    assert.equal(parseKm(' 120.500 '), 120500);
    assert.equal(parseKm('5000001'), 'invalid');
    assert.equal(parseKm('-3'), 'invalid');
  });
});

describe('parts', () => {
  it('limits the chosen parts to the product category', () => {
    assert.deepEqual(partsForProduct(['body_kaput', 'window_on_cam'], ['body_kaput', 'body_tavan']), ['body_kaput']);
    assert.deepEqual(partsForProduct(['body_kaput'], null), []);
  });

  it('collects the parts of the items once', () => {
    assert.deepEqual(
      partsFromItems([{ applied_parts: ['body_kaput', 'body_tavan'] }, { applied_parts: ['body_tavan'] }, {}]),
      ['body_kaput', 'body_tavan'],
    );
  });

  it('reads the route param', () => {
    assert.deepEqual(parseParts('body_kaput, body_tavan,'), ['body_kaput', 'body_tavan']);
    assert.deepEqual(parseParts(undefined), []);
    assert.deepEqual(parseParts(['a', 'b']), ['a', 'b']);
  });
});

describe('summary', () => {
  it('offers the current status and the transitions, never cancel', () => {
    assert.deepEqual(finishChoices('draft', ['pending', 'completed', 'cancelled']), ['draft', 'pending', 'completed']);
    assert.deepEqual(finishChoices('draft', []), ['draft']);
  });

  it('blocks completing without items', () => {
    assert.equal(finishBlocker('completed', 0), 'needs_items');
    assert.equal(finishBlocker('completed', 1), null);
    assert.equal(finishBlocker('pending', 0), null);
  });

  it('validates the number and km', () => {
    const f = summaryFromService(base);
    assert.equal(summaryError(f), null);
    assert.equal(summaryError({ ...f, serviceNo: 'ab' }), 'service_no');
    assert.equal(summaryError({ ...f, km: 'x' }), 'km');
  });

  it('patches only what changed', () => {
    const f = summaryFromService(base);
    assert.equal(servicePatch(base, f), null);
    assert.deepEqual(servicePatch(base, { ...f, notes: ' cam filmi ', km: '' }), { km: null, notes: 'cam filmi' });
    assert.deepEqual(servicePatch(base, { ...f, serviceNo: '1EVMEEI9', measure: true }), {
      service_no: '1EVMEEI9',
      has_measurement: true,
    });
    // Same number, a locked number or a measurement without VIN change nothing.
    assert.equal(servicePatch(base, { ...f, serviceNo: 'DS12AB34CD' }), null);
    assert.equal(servicePatch({ ...base, service_no_editable: false }, { ...f, serviceNo: 'X1234' }), null);
    assert.equal(servicePatch({ ...base, vin: null }, { ...f, measure: true }), null);
  });
});
