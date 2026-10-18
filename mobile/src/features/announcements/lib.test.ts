import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { isSafeUrl, isUnread, parseInline, previewOf, toBlocks } from './lib.ts';

const texts = (b: ReturnType<typeof toBlocks>) => b.map((x) => x.text);

describe('announcement lib', () => {
  it('parses markdown into blocks', () => {
    const b = toBlocks('# Title\n\nHello **world** and [link](http://x.io)\nmore\n\n- one\n- two', 'markdown');
    assert.deepEqual(
      b.map((x) => [x.kind, x.text]),
      [
        ['heading', 'Title'],
        ['paragraph', 'Hello world and link more'],
        ['bullet', 'one'],
        ['bullet', 'two'],
      ],
    );
  });

  it('reduces html to text blocks', () => {
    const b = toBlocks('<h2>Hi</h2><p>A &amp; B<br>next</p><ul><li>x</li></ul>', 'html');
    assert.deepEqual(texts(b), ['Hi', 'A & B next', 'x']);
  });

  it('drops script and style with their content', () => {
    const b = toBlocks('<style>p{color:red}</style><p>Hi</p><script>alert(1)</script>', 'html');
    assert.deepEqual(texts(b), ['Hi']);
  });

  it('decodes numeric entities', () => {
    assert.deepEqual(texts(toBlocks('<p>it&#x27;s &#8217;ok&#8217; &#39;</p>', 'html')), ['it\'s ’ok’ \'']);
  });

  it('keeps link urls from markdown and html', () => {
    const md = parseInline('see [docs](https://a.io/x) now');
    assert.deepEqual(md, [{ text: 'see ' }, { text: 'docs', url: 'https://a.io/x' }, { text: ' now' }]);
    const html = toBlocks('<p>go <a href="https://b.io/?a=1&amp;b=2">here</a></p>', 'html');
    assert.deepEqual(html[0]!.parts.find((p) => p.url), { text: 'here', url: 'https://b.io/?a=1&b=2' });
  });

  it('only allows http, https and mailto', () => {
    assert.equal(isSafeUrl('https://a.io'), true);
    assert.equal(isSafeUrl('mailto:a@b.io'), true);
    assert.equal(isSafeUrl('javascript:alert(1)'), false);
    assert.equal(isSafeUrl('tel:123'), false);
    assert.deepEqual(parseInline('[x](javascript:alert(1))').filter((p) => p.url), []);
    assert.equal(parseInline('[x](javascript:evil)')[0]!.url, undefined);
  });

  it('keeps underscores inside words', () => {
    assert.deepEqual(texts(toBlocks('foo_bar_baz and _em_ here', 'markdown')), ['foo_bar_baz and em here']);
  });

  it('keeps ordered list numbers', () => {
    const b = toBlocks('1. first\n2. second', 'markdown');
    assert.deepEqual(
      b.map((x) => [x.marker, x.text]),
      [
        ['1.', 'first'],
        ['2.', 'second'],
      ],
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
