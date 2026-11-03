import assert from 'node:assert/strict';
import { existsSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it } from 'node:test';

import { links, SCHEME } from './links.ts';

const APP_DIR = join(import.meta.dirname, '../../app/(app)');

/** Expo Router çözümü: `(grup)` klasörleri şeffaf, `[param]` her parçayı alır. */
function resolves(dir: string, segments: string[]): boolean {
  const entries = readdirSync(dir);
  const isDir = (name: string) => statSync(join(dir, name)).isDirectory();
  if (segments.length === 0) {
    if (entries.includes('index.tsx')) return true;
    return entries.some((n) => /^\(.+\)$/.test(n) && isDir(n) && resolves(join(dir, n), segments));
  }
  const [head, ...rest] = segments;
  for (const name of entries) {
    if (/^\(.+\)$/.test(name) && isDir(name) && resolves(join(dir, name), segments)) return true;
    const dynamic = /^\[.+\]$/.test(name.replace(/\.tsx$/, ''));
    const base = name.replace(/\.tsx$/, '');
    if (isDir(name)) {
      if ((name === head || (dynamic && /^\[.+\]$/.test(name))) && resolves(join(dir, name), rest)) return true;
    } else if (name.endsWith('.tsx') && rest.length === 0 && (base === head || /^\[.+\]$/.test(base))) {
      return true;
    }
  }
  return false;
}

function routeExists(link: string): boolean {
  assert.ok(link.startsWith(SCHEME), link);
  const path = link.slice(SCHEME.length);
  return resolves(APP_DIR, path === '' ? [] : path.split('/'));
}

describe('links', () => {
  const U = '11111111-1111-4111-8111-111111111111';
  const all: Record<string, string> = {
    home: links.home(''),
    vehicle: links.vehicle(U),
    service: links.service(U),
    bayi: links.bayi.home(),
    scan: links.bayi.scan(),
    newService: links.bayi.newService(),
    measure: links.bayi.measure(),
    customers: links.bayi.customers(),
    services: links.bayi.services(),
    bayiService: links.bayi.service(U),
    reports: links.bayi.reports(),
    claims: links.bayi.claims(),
    warranties: links.bayi.warranties(),
    accounting: links.bayi.accounting(),
  };

  it('yollar sözleşmedeki gibi', () => {
    assert.equal(all.home, 'olexfilms://');
    assert.equal(all.vehicle, `olexfilms://musteri/vehicle/${U}`);
    assert.equal(all.service, `olexfilms://musteri/service/${U}`);
    assert.equal(all.bayiService, `olexfilms://bayi/services/${U}`);
    assert.equal(all.claims, 'olexfilms://bayi/menu/warranty_claims');
  });

  for (const [name, link] of Object.entries(all)) {
    it(`${name} bir rota dosyasına çıkar`, () => {
      assert.ok(existsSync(APP_DIR));
      assert.ok(routeExists(link), `${link} için src/app/(app) altında dosya yok`);
    });
  }

  it('olmayan yol bulunmaz (test kendini sınar)', () => {
    assert.equal(routeExists('olexfilms://bayi/olmayan/yol'), false);
  });
});
