import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { DEFAULT_SCOPE, initialScope } from './scope.ts';

describe('service list scope of the center', () => {
  it('opens on the network so dealer names show', () => {
    assert.equal(DEFAULT_SCOPE, 'network');
    assert.equal(initialScope(null), 'network');
    assert.equal(initialScope(undefined), 'network');
    assert.equal(initialScope('garbage'), 'network');
  });

  it('keeps a saved choice', () => {
    assert.equal(initialScope('own'), 'own');
    assert.equal(initialScope('network'), 'network');
  });
});
