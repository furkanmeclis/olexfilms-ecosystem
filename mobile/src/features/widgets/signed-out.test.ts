import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { WIDGET_NAMES } from './config.ts';
import type { TFn } from './format.ts';
import { signedOutProps, signedOutTitleKey } from './signed-out.ts';
import { WIDGET_THEME } from './theme.ts';

const t: TFn = (key) => `t|${key}`;
const extras = { rtl: false, markUri: 'file:///g/olex-mark.png', markMonoUri: 'file:///g/olex-mark-mono.png', eyebrow: 'Başlık' };

describe('signedOutProps', () => {
  it('anahtar seti yalnız ortak alanlar ve bilgi kartıdır', () => {
    const props = signedOutProps(t, 'customer', extras);
    assert.deepEqual(Object.keys(props).sort(), [
      'card', 'markMonoUri', 'markUri', 'rtl', 'staleLabel', 'state', 'theme', 'updatedLabel', 'url',
    ]);
    assert.deepEqual(Object.keys(props.card ?? {}).sort(), ['cta', 'eyebrow', 'title']);
  });

  it('durum signed_out, tarih ve bayat metni boş', () => {
    const props = signedOutProps(t, 'dealer', extras);
    assert.equal(props.state, 'signed_out');
    assert.equal(props.updatedLabel, '');
    assert.equal(props.staleLabel, null);
    assert.equal(props.theme, WIDGET_THEME);
  });

  it('başlık hedef kitleye, düğme giriş metnine bağlı', () => {
    assert.equal(signedOutProps(t, 'customer', extras).card?.title, 't|state.customer_signed_out_title');
    assert.equal(signedOutProps(t, 'dealer', extras).card?.title, 't|state.dealer_signed_out_title');
    assert.equal(signedOutProps(t, 'dealer', extras).card?.cta, 't|state.sign_in');
    assert.equal(signedOutProps(t, 'dealer', extras).card?.eyebrow, 'Başlık');
  });

  it('RTL ve görsel yolları geçer, bağlantı uygulama köküdür', () => {
    const props = signedOutProps(t, 'customer', { ...extras, rtl: true });
    assert.equal(props.rtl, true);
    assert.equal(props.markUri, extras.markUri);
    assert.equal(props.markMonoUri, extras.markMonoUri);
    assert.equal(props.url, 'olexfilms://');
  });

  it('hassas alan taşımaz: yalnız çeviri anahtarları ve verilen yollar', () => {
    // Tema jetonlarında `plate` rengi var; veri alanları aranır.
    const { theme: _theme, ...data } = signedOutProps(t, 'customer', extras);
    const json = JSON.stringify(data);
    for (const word of ['plate', 'phone', 'email', 'token', 'uuid', 'vin']) assert.ok(!json.toLowerCase().includes(word), word);
  });

  it('her widget için başlık anahtarı var', () => {
    for (const name of WIDGET_NAMES) assert.match(signedOutTitleKey(name), /\.(eyebrow|title)$/);
  });
});
