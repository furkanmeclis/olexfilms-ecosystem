import assert from 'node:assert/strict';
import { test } from 'node:test';

import { BUTTON, createButtonTracker, type ButtonEvent } from './button.ts';

/** Manual timer: `run` fires what is due at `now`. */
function fakeTimer() {
  let tasks: { at: number; fn: () => void; id: number }[] = [];
  let id = 0;
  let now = 0;
  return {
    timer: {
      set: (fn: () => void, ms: number) => {
        id += 1;
        tasks.push({ at: now + ms, fn, id });
        return id;
      },
      clear: (h: unknown) => {
        tasks = tasks.filter((t) => t.id !== h);
      },
    },
    advance(to: number) {
      now = to;
      const due = tasks.filter((t) => t.at <= to);
      tasks = tasks.filter((t) => t.at > to);
      for (const t of due) t.fn();
    },
    setNow(to: number) {
      now = to;
    },
  };
}

function track() {
  const t = fakeTimer();
  const events: ButtonEvent[] = [];
  const tracker = createButtonTracker((e) => events.push(e), t.timer);
  const feed = (at: number, button: boolean, air = true) => {
    t.setNow(at);
    tracker.feed({ button, air }, at);
  };
  return { t, events, feed };
}

test('a single marked air frame is a short press, sent when the marks stop', () => {
  const { t, events, feed } = track();
  feed(1000, false);
  feed(1090, true);
  feed(1180, false);
  assert.deepEqual(events, []);
  t.advance(1090 + BUTTON.endGapMs);
  assert.deepEqual(events, ['short']);
});

test('holding the button gives one long press and no short', () => {
  const { t, events, feed } = track();
  for (let at = 0; at <= 2500; at += 450) feed(at, true);
  assert.deepEqual(events, ['long']);
  t.advance(2500 + BUTTON.endGapMs);
  assert.deepEqual(events, ['long']);
});

test('air marks right after a surface contact are ignored', () => {
  const { t, events, feed } = track();
  feed(0, true, false);
  feed(300, true);
  t.advance(300 + BUTTON.endGapMs);
  assert.deepEqual(events, []);
  feed(2000, true);
  t.advance(2000 + BUTTON.endGapMs);
  assert.deepEqual(events, ['short']);
});

test('touching a surface cancels a press in progress', () => {
  const { t, events, feed } = track();
  feed(0, true);
  feed(200, false, false);
  t.advance(200 + BUTTON.endGapMs);
  assert.deepEqual(events, []);
});

test('two separate taps are two short presses', () => {
  const { t, events, feed } = track();
  feed(0, true);
  t.advance(BUTTON.endGapMs);
  feed(2000, true);
  t.advance(2000 + BUTTON.endGapMs);
  assert.deepEqual(events, ['short', 'short']);
});
