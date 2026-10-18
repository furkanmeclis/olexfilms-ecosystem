import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { isUnread, previewOf, toBlocks } from './lib.ts';

describe('announcement lib', () => {
  it('parses markdown into blocks', () => {
    const b = toBlocks('# Title\n\nHello **world** and [link](http://x)\nmore\n\n- one\n- two', 'markdown');
    assert.deepEqual(b, [
      { kind: 'heading', text: 'Title' },
      { kind: 'paragraph', text: 'Hello world and link more' },
      { kind: 'bullet', text: 'one' },
      { kind: 'bullet', text: 'two' },
    ]);
  });

  it('reduces html to text blocks', () => {
    const b = toBlocks('<h2>Hi</h2><p>A &amp; B<br>next</p><ul><li>x</li></ul>', 'html');
    assert.deepEqual(
      b.map((x) => x.text),
      ['Hi', 'A & B next', 'x'],
    );
  });

  it('truncates previews', () => {
    assert.equal(previewOf('a'.repeat(300), 'markdown', 10).length, 10);
    assert.equal(previewOf('short', 'markdown'), 'short');
  });

  it('flags unread', () => {
    assert.equal(isUnread({ read_at: null }), true);
    assert.equal(isUnread({}), true);
    assert.equal(isUnread({ read_at: '2026-10-19T00:00:00Z' }), false);
  });
});
