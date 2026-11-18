import assert from 'node:assert/strict';
import { afterEach, beforeEach, describe, it, mock } from 'node:test';

import { describeError, type Translator } from './error-text.ts';
import { createToaster, DURATIONS, MAX_TOASTS } from './store.ts';

describe('toast store (TEC-1235)', () => {
  beforeEach(() => mock.timers.enable({ apis: ['setTimeout'] }));
  afterEach(() => mock.timers.reset());

  const list = (t: ReturnType<typeof createToaster>) => t.store.getState().toasts;

  it('puts the newest on top and keeps at most MAX_TOASTS', () => {
    const t = createToaster();
    for (let i = 1; i <= MAX_TOASTS + 2; i++) t.info(`m${i}`);
    assert.equal(MAX_TOASTS, 3);
    assert.deepEqual(
      list(t).map((x) => x.message),
      ['m5', 'm4', 'm3'],
    );
  });

  it('dedupes the same kind + message: refreshes the timer instead of stacking', () => {
    const t = createToaster();
    const a = t.error('Kaydedilemedi');
    mock.timers.tick(DURATIONS.error - 100);
    const b = t.error('Kaydedilemedi');
    assert.equal(a, b);
    assert.equal(list(t).length, 1);
    assert.equal(list(t)[0].version, 2);
    // The first timer was cleared: still visible past the original deadline.
    mock.timers.tick(200);
    assert.equal(list(t).length, 1);
    mock.timers.tick(DURATIONS.error);
    assert.equal(list(t).length, 0);
  });

  it('does not dedupe across kinds', () => {
    const t = createToaster();
    t.error('x');
    t.warning('x');
    assert.equal(list(t).length, 2);
  });

  it('auto-dismisses after the per-kind duration', () => {
    const t = createToaster();
    t.success('ok');
    t.warning('dikkat');
    mock.timers.tick(DURATIONS.success);
    assert.deepEqual(
      list(t).map((x) => x.kind),
      ['warning'],
    );
    mock.timers.tick(DURATIONS.warning - DURATIONS.success);
    assert.equal(list(t).length, 0);
  });

  it('keeps loading until it is replaced by the same id', () => {
    const t = createToaster();
    const id = t.loading('Gönderiliyor');
    t.info('başka');
    mock.timers.tick(60_000);
    assert.equal(list(t).length, 1);
    assert.equal(list(t)[0].kind, 'loading');

    assert.equal(t.success('Gönderildi', { id, description: 'D-12' }), id);
    assert.equal(list(t).length, 1);
    assert.deepEqual(
      { kind: list(t)[0].kind, message: list(t)[0].message, description: list(t)[0].description },
      { kind: 'success', message: 'Gönderildi', description: 'D-12' },
    );
    mock.timers.tick(DURATIONS.success);
    assert.equal(list(t).length, 0);
  });

  it('replaces in place, keeping the stack order', () => {
    const t = createToaster();
    const id = t.loading('a');
    t.info('b');
    t.error('c', { id });
    assert.deepEqual(
      list(t).map((x) => `${x.kind}:${x.message}`),
      ['info:b', 'error:c'],
    );
  });

  it('dismisses one toast or all of them, clearing their timers', () => {
    const t = createToaster();
    const a = t.success('a');
    t.success('b');
    t.dismiss(a);
    assert.deepEqual(
      list(t).map((x) => x.message),
      ['b'],
    );
    t.dismiss('unknown');
    assert.equal(list(t).length, 1);
    t.dismiss();
    assert.equal(list(t).length, 0);
    mock.timers.tick(DURATIONS.success);
    assert.equal(list(t).length, 0);
  });

  it('honours an explicit duration and keeps the action', () => {
    const t = createToaster();
    let undone = false;
    t.success('Silindi', { duration: 1000, action: { label: 'Geri al', onPress: () => (undone = true) } });
    list(t)[0].action?.onPress();
    assert.equal(undone, true);
    mock.timers.tick(1000);
    assert.equal(list(t).length, 0);
  });

  it('promise: loading, then success or error on the same toast', async () => {
    mock.timers.reset();
    const t = createToaster();
    await t.promise(Promise.resolve(3), { loading: 'l', success: (n) => `ok ${n}`, error: 'fail' });
    assert.deepEqual(
      list(t).map((x) => `${x.kind}:${x.message}`),
      ['success:ok 3'],
    );
    await t.promise(Promise.reject(new Error('x')), { loading: 'l2', success: 'ok', error: 'fail' }).catch(() => {});
    await Promise.resolve();
    assert.deepEqual(
      list(t).map((x) => `${x.kind}:${x.message}`),
      ['error:fail', 'success:ok 3'],
    );
    t.dismiss();
  });
});

describe('describeError (toast.apiError mapping)', () => {
  const catalog: Record<string, string> = {
    'common:error_generic': 'Bir şeyler ters gitti. Tekrar dene.',
    'common:error_network': 'Bağlantı yok. İnternetini kontrol edip tekrar dene.',
    'errors:codes.NOT_FOUND': 'Kayıt bulunamadı.',
    'warehouse:errors.STOCK_LOCKED': 'Stok sayımda, şimdi değiştirilemez.',
  };
  const tr: Translator = { exists: (k) => k in catalog, t: (k) => catalog[k] ?? k };
  const apiError = (code: string) => Object.assign(new Error(`HTTP 500 ${code}`), { name: 'ApiError', code });

  it('maps a known code through the errors catalog', () => {
    assert.equal(describeError(apiError('NOT_FOUND'), tr), 'Kayıt bulunamadı.');
  });
  it('says the connection is missing for NETWORK_ERROR', () => {
    assert.equal(describeError(apiError('NETWORK_ERROR'), tr), catalog['common:error_network']);
  });
  it('checks feature catalogs first', () => {
    assert.equal(describeError(apiError('STOCK_LOCKED'), tr, { namespaces: ['warehouse'] }), catalog['warehouse:errors.STOCK_LOCKED']);
  });
  it('never shows a raw code: unknown codes and non-API errors fall back', () => {
    assert.equal(describeError(apiError('SOME_INTERNAL_CODE'), tr), catalog['common:error_generic']);
    assert.equal(describeError(new Error('TypeError: x is undefined'), tr), catalog['common:error_generic']);
    assert.equal(describeError('boom', tr, { fallback: 'Dışa aktarılamadı' }), 'Dışa aktarılamadı');
  });
});
