/**
 * Server-sent events of the assistant (TEC-388 stream: message_start,
 * text_delta, tool_start, tool_result, confirm, action, quota_exceeded,
 * error, message_done, title; `: ping` comments). Pure functions with no
 * React Native import, so `node --test` covers them.
 *
 * EventSource cannot POST and is not in React Native, so the app reads the
 * `expo/fetch` body stream and feeds the decoded text to `createSseParser`.
 */

export type SseEvent = { event: string; data: unknown };

/**
 * Parses one event block (lines up to a blank line). Comment lines and
 * blocks without `data` are skipped; `data` lines join with "\n" and are
 * parsed as JSON (invalid JSON drops the block).
 */
export function parseEventBlock(block: string): SseEvent | null {
  let event = 'message';
  const data: string[] = [];
  for (const raw of block.split('\n')) {
    const line = raw.endsWith('\r') ? raw.slice(0, -1) : raw;
    if (!line || line.startsWith(':')) continue;
    const colon = line.indexOf(':');
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'event') event = value;
    else if (field === 'data') data.push(value);
  }
  if (data.length === 0) return null;
  try {
    return { event, data: JSON.parse(data.join('\n')) };
  } catch {
    return null;
  }
}

/**
 * Incremental parser: `push` any slice of the decoded text (chunks may cut
 * lines, CRLF pairs or events anywhere); every complete event goes to
 * `onEvent` in order. `end` flushes a last event without a trailing blank
 * line. The whole body at once (the non-streaming fallback) works the same.
 */
export function createSseParser(onEvent: (event: SseEvent) => void) {
  let buffer = '';
  const drain = () => {
    let index = buffer.indexOf('\n\n');
    while (index !== -1) {
      const parsed = parseEventBlock(buffer.slice(0, index));
      buffer = buffer.slice(index + 2);
      if (parsed) onEvent(parsed);
      index = buffer.indexOf('\n\n');
    }
  };
  return {
    push(text: string) {
      if (!text) return;
      // A "\r" at the end of the buffer may pair with a "\n" of this chunk.
      buffer = (buffer + text).replace(/\r\n/g, '\n');
      drain();
    },
    end() {
      buffer = buffer.replace(/\r\n/g, '\n').replace(/\r$/, '');
      drain();
      const rest = buffer.trim() ? parseEventBlock(buffer) : null;
      buffer = '';
      if (rest) onEvent(rest);
    },
  };
}

/**
 * Streaming UTF-8 decoder: a multi-byte character split across chunks is
 * kept until its last byte arrives. Uses the platform TextDecoder when there
 * is one (Hermes ships it) and a small pure decoder otherwise.
 */
export function createUtf8Decoder(): { decode(bytes?: Uint8Array): string } {
  if (typeof TextDecoder !== 'undefined') {
    const d = new TextDecoder('utf-8');
    return { decode: (bytes) => (bytes ? d.decode(bytes, { stream: true }) : d.decode()) };
  }
  return createPureUtf8Decoder();
}

/** Pure decoder behind `createUtf8Decoder` (exported for tests). */
export function createPureUtf8Decoder(): { decode(bytes?: Uint8Array): string } {
  let pending: number[] = [];
  return {
    decode(bytes) {
      const input = bytes ? [...pending, ...bytes] : pending;
      pending = [];
      let out = '';
      let i = 0;
      while (i < input.length) {
        const b = input[i];
        const need = b < 0x80 ? 0 : b >= 0xf8 ? -1 : b >= 0xf0 ? 3 : b >= 0xe0 ? 2 : b >= 0xc0 ? 1 : -1;
        if (need === -1) {
          out += '�';
          i += 1;
          continue;
        }
        if (i + need >= input.length && need > 0 && bytes) {
          // Incomplete sequence at the end of a chunk: wait for the rest.
          pending = input.slice(i);
          break;
        }
        if (i + need >= input.length && need > 0) {
          out += '�';
          break;
        }
        let cp = need === 0 ? b : b & (0x3f >> need);
        let ok = true;
        for (let k = 1; k <= need; k++) {
          const c = input[i + k];
          if ((c & 0xc0) !== 0x80) {
            ok = false;
            break;
          }
          cp = (cp << 6) | (c & 0x3f);
        }
        if (!ok) {
          out += '�';
          i += 1;
          continue;
        }
        out += String.fromCodePoint(cp);
        i += need + 1;
      }
      return out;
    },
  };
}
