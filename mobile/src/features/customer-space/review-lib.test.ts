import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { reviewMissing } from './review-lib.ts';

describe('reviewMissing', () => {
  it('asks for both star ratings first, then the required questions', () => {
    assert.equal(reviewMissing({ platform: 0, product: 0, requiredUnanswered: 2 }), 'ratings');
    assert.equal(reviewMissing({ platform: 5, product: 0, requiredUnanswered: 0 }), 'ratings');
    assert.equal(reviewMissing({ platform: 5, product: 4, requiredUnanswered: 1 }), 'questions');
    assert.equal(reviewMissing({ platform: 5, product: 4, requiredUnanswered: 0 }), null);
  });
});
