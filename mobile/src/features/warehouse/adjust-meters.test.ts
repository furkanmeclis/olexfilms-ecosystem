import assert from "node:assert/strict";
import { test } from "node:test";

import { needsPhoto, normalizeMeters } from "./adjust-meters.ts";

test("normalizeMeters accepts a comma and at most two decimals", () => {
  assert.equal(normalizeMeters("2,5"), "2.5");
  assert.equal(normalizeMeters(" 12.25 "), "12.25");
  assert.equal(normalizeMeters("0"), undefined);
  assert.equal(normalizeMeters("1.234"), undefined);
  assert.equal(normalizeMeters("x"), undefined);
});

test("a damage document needs a photo", () => {
  assert.equal(needsPhoto({ type: "damage", photos: [] }), true);
  assert.equal(needsPhoto({ type: "damage", photos: [{}] }), false);
  assert.equal(needsPhoto({ type: "loss", photos: [] }), false);
});
