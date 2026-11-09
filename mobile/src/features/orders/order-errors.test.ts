import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it } from 'node:test';

/**
 * TEC-1061: a dealer order with a product closed to dealer orders fails with
 * PRODUCT_NOT_ORDERABLE. The order screens show it through errorMessage(),
 * which reads the web's errors catalog (errors:codes.*), so every app
 * language must carry its own text there.
 */
describe('PRODUCT_NOT_ORDERABLE text', () => {
  const MOBILE = fileURLToPath(new URL('../../core/i18n/locales/', import.meta.url));
  const WEB = fileURLToPath(new URL('../../../../frontend/src/locales/', import.meta.url));
  const langs = readdirSync(MOBILE).filter((l) => !l.startsWith('.'));
  const text = (lang: string) =>
    (JSON.parse(readFileSync(join(WEB, lang, 'errors.json'), 'utf8')) as Record<string, string>)[
      'codes.PRODUCT_NOT_ORDERABLE'
    ];

  it('covers all 13 app languages', () => {
    assert.equal(langs.length, 13);
  });

  it('is translated in every app language', () => {
    const en = text('en');
    assert.ok(en && en.trim().length > 0);
    for (const lang of langs) {
      const v = text(lang);
      assert.ok(v && v.trim().length > 0, `${lang}: missing`);
      if (lang !== 'en') assert.notEqual(v, en, `${lang}: English text`);
    }
  });
});
