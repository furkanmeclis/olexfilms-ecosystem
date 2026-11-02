import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import type { FeedbackMeta } from './meta.ts';
import { buildZipPlan, recordFolder, zipFileName } from './zip-plan.ts';

function record(id: string, created_at: string, annotated: boolean): FeedbackMeta {
  return {
    id,
    created_at,
    timezone: 'Europe/Istanbul',
    utc_offset_minutes: 180,
    note: 'n',
    files: { original: 'original.png', annotated: annotated ? 'annotated.png' : null },
    location: { pathname: '/bayi', segments: [], route_params: {}, query_param_keys: [], history: [], space: 'bayi' },
    session: {
      session_kind: 'staff',
      user_uuid: 'u',
      organization_uuid: null,
      organization_type: null,
      membership_role: null,
      roles: [],
      permission_count: 0,
    },
    app: {
      version: '2.0.0',
      build: '1',
      bundle_id: 'x',
      ota_enabled: false,
      ota_channel: null,
      ota_runtime_version: null,
      ota_update_id: null,
      ota_embedded: null,
      environment: 'production',
      api_url: 'https://h.test/api',
    },
    device: {
      brand: null,
      model: null,
      os: null,
      os_version: null,
      screen: { width: 1, height: 1, scale: 1, font_scale: 1 },
      language: 'tr',
      rtl: false,
      theme_preference: 'system',
      color_scheme: null,
      network: null,
      battery: null,
    },
    api_errors: [],
  };
}

describe('zip plan', () => {
  const a = record('aaa', '2026-11-01T10:00:00.000Z', true);
  const b = record('bbb', '2026-11-01T09:00:00.000Z', false);
  const plan = buildZipPlan([a, b], { app: { version: '2.0.0' } }, new Date('2026-11-01T12:00:00Z'));

  it('lists feedback.json first, then one folder per record, oldest first', () => {
    assert.deepEqual(
      plan.map((e) => e.path),
      [
        'feedback.json',
        '001-bbb/original.png',
        '001-bbb/meta.json',
        '002-aaa/original.png',
        '002-aaa/annotated.png',
        '002-aaa/meta.json',
      ],
    );
  });

  it('points image entries at the record files on the device', () => {
    const files = plan.filter((e) => e.kind === 'file');
    assert.deepEqual(
      files.map((e) => (e.kind === 'file' ? `${e.recordId}/${e.file}` : '')),
      ['bbb/original.png', 'aaa/original.png', 'aaa/annotated.png'],
    );
  });

  it('writes ZIP paths into meta.json and null when there is no drawing', () => {
    const metaB = plan.find((e) => e.path === '001-bbb/meta.json');
    if (metaB?.kind !== 'json') throw new Error('meta.json missing');
    assert.deepEqual((metaB.data as FeedbackMeta).files, { original: '001-bbb/original.png', annotated: null });
  });

  it('summarizes every record and the device in feedback.json', () => {
    const head = plan[0];
    if (head.kind !== 'json') throw new Error('feedback.json is not first');
    const data = head.data as { count: number; records: { folder: string }[]; device: unknown; exported_at: string };
    assert.equal(data.count, 2);
    assert.deepEqual(data.records.map((r) => r.folder), ['001-bbb', '002-aaa']);
    assert.deepEqual(data.device, { app: { version: '2.0.0' } });
    assert.equal(data.exported_at, '2026-11-01T12:00:00.000Z');
  });

  it('keeps forbidden keys out even if a record carries one', () => {
    const dirty = { ...a, token: 'secret' } as FeedbackMeta;
    const p = buildZipPlan([dirty], {}, new Date());
    assert.ok(!JSON.stringify(p).includes('secret'));
  });

  it('handles no records', () => {
    assert.deepEqual(buildZipPlan([], {}, new Date()).map((e) => e.path), ['feedback.json']);
  });

  it('names the ZIP and folders safely', () => {
    assert.equal(zipFileName(new Date(2026, 10, 1, 9, 5)), 'olexfilms-feedback-2026-11-01-0905.zip');
    assert.equal(recordFolder(12, '../x'), '012-___x');
  });
});
