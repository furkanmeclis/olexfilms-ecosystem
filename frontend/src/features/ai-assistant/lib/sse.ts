import type { StreamEvent } from "./types";

/**
 * Parses one `text/event-stream` block (lines up to a blank line). Comment
 * lines (`: ping`) and blocks without data are skipped; `data` is JSON.
 */
export function parseEventBlock(block: string): StreamEvent | null {
  let event = "message";
  const data: string[] = [];
  for (const raw of block.split("\n")) {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    if (!line || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") event = value;
    else if (field === "data") data.push(value);
  }
  if (data.length === 0) return null;
  try {
    return { event, data: JSON.parse(data.join("\n")) } as StreamEvent;
  } catch {
    return null;
  }
}

/**
 * Reads a server-sent event stream (fetch + ReadableStream; EventSource
 * cannot POST) and calls `onEvent` for every event in order. Resolves when
 * the stream ends; an aborted fetch rejects with AbortError.
 */
export async function readEventStream(
  stream: ReadableStream<Uint8Array>,
  onEvent: (event: StreamEvent) => void,
): Promise<void> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (value) buffer += decoder.decode(value, { stream: true });
      if (done) buffer += decoder.decode();
      buffer = buffer.replace(/\r\n/g, "\n");
      let index = buffer.indexOf("\n\n");
      while (index !== -1) {
        const parsed = parseEventBlock(buffer.slice(0, index));
        buffer = buffer.slice(index + 2);
        if (parsed) onEvent(parsed);
        index = buffer.indexOf("\n\n");
      }
      if (done) {
        const parsed = buffer.trim() ? parseEventBlock(buffer) : null;
        if (parsed) onEvent(parsed);
        return;
      }
    }
  } finally {
    reader.releaseLock();
  }
}
