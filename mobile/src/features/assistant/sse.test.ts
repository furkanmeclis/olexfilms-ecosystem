import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { createPureUtf8Decoder, createSseParser, createUtf8Decoder, parseEventBlock, type SseEvent } from './sse.ts';

function collect(chunks: string[]) {
  const out: SseEvent[] = [];
  const p = createSseParser((e) => out.push(e));
  for (const c of chunks) p.push(c);
  p.end();
  return out;
}

const STREAM =
  ': stream open\n\n' +
  'event: message_start\ndata: {"conversation_uuid":"c","message_uuid":"m"}\n\n' +
  ': ping\n\n' +
  'event: text_delta\ndata: {"text":"Merhaba "}\n\n' +
  'event: text_delta\ndata: {"text":"dünya"}\n\n' +
  'event: message_done\ndata: {"message_uuid":"m","status":"complete"}\n\n';

describe('parseEventBlock', () => {
  it('reads event and JSON data', () => {
    assert.deepEqual(parseEventBlock('event: title\ndata: {"title":"x"}'), { event: 'title', data: { title: 'x' } });
  });
  it('defaults the event name to message and joins data lines', () => {
    assert.deepEqual(parseEventBlock('data: [1,\ndata: 2]'), { event: 'message', data: [1, 2] });
  });
  it('skips comments, empty blocks and invalid JSON', () => {
    assert.equal(parseEventBlock(': ping'), null);
    assert.equal(parseEventBlock(''), null);
    assert.equal(parseEventBlock('event: x\ndata: {nope'), null);
  });
  it('keeps a value without the optional space and strips CR', () => {
    assert.deepEqual(parseEventBlock('event:error\r\ndata:{"code":"x"}\r'), { event: 'error', data: { code: 'x' } });
  });
});

describe('createSseParser', () => {
  it('parses a whole body at once (non-streaming fallback)', () => {
    const events = collect([STREAM]);
    assert.deepEqual(
      events.map((e) => e.event),
      ['message_start', 'text_delta', 'text_delta', 'message_done'],
    );
    assert.deepEqual(events[2].data, { text: 'dünya' });
  });

  it('gives the same events however the body is chunked', () => {
    const whole = collect([STREAM]);
    for (const size of [1, 2, 3, 7, 16]) {
      const chunks: string[] = [];
      for (let i = 0; i < STREAM.length; i += size) chunks.push(STREAM.slice(i, i + size));
      assert.deepEqual(collect(chunks), whole, `chunk size ${size}`);
    }
  });

  it('handles CRLF split across chunks', () => {
    const crlf = STREAM.replace(/\n/g, '\r\n');
    const chunks: string[] = [];
    for (let i = 0; i < crlf.length; i += 5) chunks.push(crlf.slice(i, i + 5));
    assert.deepEqual(collect(chunks), collect([STREAM]));
  });

  it('flushes a last event without a trailing blank line', () => {
    assert.deepEqual(collect(['event: title\ndata: {"title":"t"}']), [{ event: 'title', data: { title: 't' } }]);
  });

  it('emits complete events before the stream ends', () => {
    const out: SseEvent[] = [];
    const p = createSseParser((e) => out.push(e));
    p.push('event: text_delta\ndata: {"text":"a"}\n\nevent: text_');
    assert.equal(out.length, 1);
    p.push('delta\ndata: {"text":"b"}\n\n');
    assert.equal(out.length, 2);
  });
});

describe('UTF-8 decoding', () => {
  const bytes = new TextEncoder().encode('Çalışıyor… 🚗 ok');

  for (const [name, make] of [
    ['pure', createPureUtf8Decoder],
    ['platform', createUtf8Decoder],
  ] as const) {
    it(`${name}: keeps characters split across chunks`, () => {
      for (const size of [1, 2, 3, 5]) {
        const d = make();
        let text = '';
        for (let i = 0; i < bytes.length; i += size) text += d.decode(bytes.slice(i, i + size));
        text += d.decode();
        assert.equal(text, 'Çalışıyor… 🚗 ok', `chunk size ${size}`);
      }
    });
  }

  it('pure: replaces invalid bytes and a truncated tail', () => {
    const d = createPureUtf8Decoder();
    assert.equal(d.decode(new Uint8Array([0x61, 0xff, 0x62])), 'a�b');
    assert.equal(d.decode(new Uint8Array([0xe2, 0x80])), '');
    assert.equal(d.decode(), '�');
  });
});
