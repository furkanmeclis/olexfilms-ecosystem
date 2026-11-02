import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { apiErrorPath, clearApiErrors, logApiError, recentApiErrors } from '../../core/api/error-log.ts';
import { isForbiddenKey, pushHistory, sanitizeMeta, splitRouteParams, stripQuery } from './meta.ts';

const SECRET = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlLXNpZ24';

describe('sanitizeMeta', () => {
  it('drops forbidden keys at any depth', () => {
    const dirty = {
      note: 'ok',
      access_token: SECRET,
      session: {
        user_uuid: 'u1',
        refreshToken: 'r',
        headers: { Authorization: `Bearer ${SECRET}`, Cookie: 'sid=1', 'x-api-key': 'k' },
      },
      password: 'hunter2',
      list: [{ body: '{"a":1}', request_body: 'x', response_body: 'y', ok: 1 }],
      query: { q: 'secret search' },
      session_id: 's',
      client_secret: 'c',
    };
    const out = sanitizeMeta(dirty) as Record<string, unknown>;
    const json = JSON.stringify(out);
    for (const bad of [SECRET, 'hunter2', 'sid=1', '"r"', '{\\"a\\":1}', 'secret search', 'Authorization', 'Cookie']) {
      assert.ok(!json.includes(bad), `leaked ${bad}`);
    }
    assert.equal((out.session as Record<string, unknown>).user_uuid, 'u1');
    assert.deepEqual(out.list, [{ ok: 1 }]);
  });

  it('keeps the regular metadata fields', () => {
    for (const k of ['session_kind', 'user_uuid', 'permission_count', 'api_url', 'pathname', 'route_params', 'query_param_keys', 'api_errors', 'status', 'path', 'method']) {
      assert.equal(isForbiddenKey(k), false, k);
    }
  });

  it('strips query strings from paths and URLs', () => {
    const out = sanitizeMeta({ api_url: 'https://x.test/api?key=1', pathname: '/bayi/services?tab=open', label: 'a?b' });
    assert.deepEqual(out, { api_url: 'https://x.test/api', pathname: '/bayi/services', label: 'a?b' });
  });

  it('scrubs bearer tokens and JWTs pasted into the note', () => {
    const out = sanitizeMeta({ note: `token was Bearer abc.def and ${SECRET}` });
    assert.ok(!out.note.includes('abc.def'));
    assert.ok(!out.note.includes(SECRET));
  });
});

describe('route params', () => {
  it('keeps dynamic segment values and only the names of other params', () => {
    const r = splitRouteParams(['(app)', 'bayi', 'services', '[uuid]'], { uuid: 'abc', tab: 'notes', q: 'plate 34' });
    assert.deepEqual(r, { route_params: { uuid: 'abc' }, query_param_keys: ['q', 'tab'] });
  });

  it('handles catch-all segments', () => {
    const r = splitRouteParams(['soon', '[...rest]'], { rest: ['a', 'b'] });
    assert.deepEqual(r.route_params, { rest: 'a/b' });
  });

  it('never keeps a token-like param value', () => {
    const r = splitRouteParams(['qr-login', '[token]'], { token: 'secret' });
    assert.deepEqual(r, { route_params: {}, query_param_keys: ['token'] });
  });
});

describe('route history', () => {
  it('keeps the last 10 distinct steps without queries', () => {
    let h: string[] = [];
    for (let i = 0; i < 15; i++) h = pushHistory(h, `/p/${i}?x=${i}`);
    h = pushHistory(h, '/p/14');
    assert.equal(h.length, 10);
    assert.equal(h[0], '/p/5');
    assert.equal(h[9], '/p/14');
    assert.equal(stripQuery('/a#frag'), '/a');
  });
});

describe('api error ring buffer', () => {
  it('keeps the last 20 failures with path only', () => {
    clearApiErrors();
    for (let i = 0; i < 25; i++) logApiError({ method: 'get', path: `/v1/mobile/panel/x/${i}?token=abc`, status: 500 });
    const e = recentApiErrors();
    assert.equal(e.length, 20);
    assert.equal(e[0].path, '/v1/mobile/panel/x/5');
    assert.equal(e[19].method, 'GET');
    assert.ok(!JSON.stringify(e).includes('token'));
  });

  it('turns request URLs into paths under the API base', () => {
    assert.equal(apiErrorPath('https://h.test/api/v1/mobile/me?x=1', 'https://h.test/api'), '/v1/mobile/me');
    assert.equal(apiErrorPath('https://h.test/v1/me', 'https://h.test'), '/v1/me');
  });
});
