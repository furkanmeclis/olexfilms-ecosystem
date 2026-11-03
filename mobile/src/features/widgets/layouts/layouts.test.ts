// Layout'lar `node:test` ile import edilemez (`.tsx`, native modül). Bu dosya kaynağı statik denetler,
// Babel ile derler ve eklentinin ürettiği metni extension'daki gibi sahte küresellerle çalıştırır.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { describe, it } from 'node:test';

import { WIDGET_NAMES } from '../config.ts';
import { stripNulls } from '../native-props.ts';
import { WIDGET_THEME } from '../theme.ts';
import type { BaseProps, KorumaProps, PanomProps, WidgetState } from '../types.ts';

const require = createRequire(import.meta.url);
const babel = require('@babel/core');

const DIR = import.meta.dirname;
const MOBILE = path.resolve(DIR, '../../../..');
const FILES = fs.readdirSync(DIR).filter((f) => f.endsWith('.widget.tsx'));
const read = (file: string) => fs.readFileSync(path.join(DIR, file), 'utf8');

const ALLOWED_IMPORTS = ['@expo/ui/swift-ui', '@expo/ui/swift-ui/modifiers', 'expo-widgets'];
/** `jsx-runtime-stub.ts` ve gövdenin kullanabileceği dil küreselleri. */
const RUNTIME_GLOBALS = ['_jsx', '_jsxs', '_jsxDEV', '_Fragment', '_jsxFileName', 'Infinity', 'undefined', 'Math', 'Number', 'String'];
const HOME = ['systemSmall', 'systemMedium', 'systemLarge'];
const LOCK = ['accessoryCircular', 'accessoryRectangular', 'accessoryInline'];
const NON_OK: WidgetState[] = ['signed_out', 'empty', 'no_permission', 'module_off', 'wrong_role'];

/** Extension'da küresel olan adlar: `ui-globals.ts`, `@expo/ui/swift-ui` ve `modifiers` dışa aktarımları. */
function expoUiNames(): Set<string> {
  const names = new Set<string>();
  const walk = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name.endsWith('.d.ts')) {
        for (const m of fs.readFileSync(full, 'utf8').matchAll(/export declare (?:function|const) (\w+)/g)) names.add(m[1]);
      }
    }
  };
  walk(path.join(MOBILE, 'node_modules/@expo/ui/build/swift-ui'));
  return names;
}

function parse(code: string, filename: string) {
  return babel.parseSync(code, {
    babelrc: false,
    configFile: false,
    filename,
    sourceType: 'module',
    parserOpts: { plugins: ['typescript', 'jsx'] },
  });
}

/** Metro'nun iOS paketlemesiyle aynı önayar. */
function compile(file: string, isDev: boolean): string {
  return babel.transformSync(read(file), {
    babelrc: false,
    configFile: false,
    cwd: MOBILE,
    filename: path.join(DIR, file),
    presets: [require.resolve('babel-preset-expo')],
    caller: { name: 'metro', bundler: 'metro', platform: 'ios', isDev, engine: 'hermes', supportsStaticESM: false },
  }).code;
}

/** Derlenen modülü sahte `createWidget` ile çalıştırır; adı ve eklentinin ürettiği metni döndürür. */
function widgetOf(file: string, isDev: boolean): { name: string; layout: string } {
  const created: { name: string; layout: string }[] = [];
  const fakeRequire = (id: string) =>
    id === 'expo-widgets'
      ? { createWidget: (name: string, layout: string) => created.push({ name, layout }) }
      : {};
  const module = { exports: {} as { create: (props: object) => void } };
  new Function('require', 'module', 'exports', compile(file, isDev))(fakeRequire, module, module.exports);
  module.exports.create({});
  assert.equal(created.length, 1, `${file}: tek createWidget`);
  return created[0];
}

type Node = { type: string; key?: unknown; props: Record<string, any> };
type Modifier = { $type: string; args: unknown[] };

/** Metni extension'daki gibi değerlendirir: `__expoWidgetLayout = (layout)`, adlar küresel. */
function evaluate(layout: string): (props: object, environment: object) => Node {
  const scope = new Proxy(
    {},
    {
      has: () => true,
      get: (_, key) => {
        if (typeof key !== 'string') return undefined;
        if (key.startsWith('_jsx')) return (type: string, props: object, k?: unknown) => ({ type, key: k, props });
        if (key === 'Infinity' || key === 'undefined' || key in Math || key === 'Math' || key === 'Number' || key === 'String') {
          return (globalThis as Record<string, unknown>)[key];
        }
        if (/^[A-Z]/.test(key)) return key;
        return (...args: unknown[]) => {
          if (key === 'frame') {
            const p = args[0] as Record<string, unknown>;
            const fixed = 'width' in p || 'height' in p;
            const flexible = Object.keys(p).some((k) => /^(min|ideal|max)/.test(k));
            assert.ok(!(fixed && flexible), 'frame sabit ve esnek değerleri karıştıramaz');
          }
          return { $type: key, args };
        };
      },
    },
  );
  return new Function('scope', `with (scope) { return (${layout}); }`)(scope);
}

/** Ağaçtaki tüm `Text` metinleri ve `Image` yolları. */
function collect(node: unknown, out: string[] = []): string[] {
  if (Array.isArray(node)) node.forEach((child) => collect(child, out));
  else if (node && typeof node === 'object') {
    const { type, props } = node as Node;
    if (type === 'Text') out.push(String(props.children));
    if (type === 'Image') out.push(String(props.uiImage));
    collect(props.children, out);
  }
  return out;
}

const modifierTypes = (node: Node) => ((node.props.modifiers ?? []) as Modifier[]).map((m) => m.$type);

const base = (state: WidgetState, rtl = false): BaseProps => ({
  state,
  rtl,
  theme: WIDGET_THEME,
  markUri: 'file:///mark.png',
  markMonoUri: 'file:///mark-mono.png',
  url: 'olexfilms://home',
  updatedLabel: 'Güncellendi 14:05',
  staleLabel: null,
});

const warranty = (name: string, warn = false) => ({
  name,
  leftLabel: '2 yıl 4 ay',
  leftLongLabel: '2 yıl 4 ay kaldı',
  leftShortLabel: '2y 4a',
  percent: warn ? 8 : 47,
  warn,
});

const korumaVehicle = (id: string, warranties: KorumaProps['vehicles'][number]['warranties']) => ({
  id,
  url: `olexfilms://vehicle/${id}`,
  title: 'Porsche 911',
  plate: '34 ABC 123',
  plateCountry: 'TR',
  heroUri: 'file:///hero.png',
  brandLogoUri: null,
  brandInitial: 'P',
  activeCountLabel: '3 aktif garanti',
  lastServiceLabel: 'Son hizmet 12 Eki 2026',
  warranties,
  emptyCard: warranties.length ? null : { eyebrow: 'Porsche 911', title: 'Aktif garanti yok', body: 'Bu araçta aktif garanti yok.' },
  locked: warranties.length
    ? { title: 'Koruman', short: '2y 4a', long: '2 yıl 4 ay kaldı', inline: 'Garanti 2 yıl 4 ay' }
    : { title: 'Koruman', short: '', long: 'Aktif garanti yok', inline: 'Aktif garanti yok' },
});

const korumaProps = (rtl: boolean): KorumaProps => ({
  ...base('ok', rtl),
  eyebrow: 'Koruman',
  leftCaption: 'kaldı',
  vehicles: [
    korumaVehicle('a', [warranty('Olex PPF Parlak'), warranty('Olex Seramik'), warranty('Olex Cam Filmi', true)]),
    korumaVehicle('b', [warranty('Tek Film')]),
    korumaVehicle('c', []),
  ],
});

const panomProps = (rtl: boolean): PanomProps => ({
  ...base('ok', rtl),
  title: 'Panom',
  next: '4',
  active: '5',
  done: '2',
  rows: [
    {
      uuid: 'u1',
      url: 'olexfilms://bayi/services/u1',
      carTitle: 'Audi A4',
      plate: '06 DEF 456',
      plateCountry: 'TR',
      meta: 'Cam filmi · 2 saat',
      statusIcon: 'clock',
      statusTone: 'progress',
      logoUri: null,
      initial: 'A',
    },
  ],
  longest: { uuid: 'u1', url: 'olexfilms://bayi/services/u1', carTitle: 'Audi A4', plate: '06 DEF 456', waitLabel: 'En uzun bekleyen · 2 saat' },
  locked: { title: 'Panom', count: '4', countLabel: 'sırada', main: '4 sırada', sub: '5 işlemde · 2 bitti', inline: 'Panom · 4 sırada, 5 işlemde' },
});

/** Veri dalları sınanan layout'lar; yeni layout buraya eklenir. */
const FIXTURES: Record<string, (rtl: boolean) => object> = {
  OlexKoruma: korumaProps,
  OlexPanom: panomProps,
};

/** Kilit ekranında hiçbir dalda görünmemesi gereken veriler. */
const PRIVATE_VALUES = ['34 ABC 123', 'Porsche', '06 DEF 456', 'Audi', 'Olex PPF Parlak', 'Tek Film', 'file:///hero.png', 'Cam filmi'];

/** Ağaçtaki tüm düğümler. */
const flatten = (node: unknown): Node[] =>
  Array.isArray(node) ? node.flatMap(flatten) : node && typeof node === 'object' ? [node as Node, ...flatten((node as Node).props.children)] : [];

const env = (widgetFamily: string, colorScheme = 'light', configuration?: object) => ({
  widgetFamily,
  colorScheme,
  widgetRenderingMode: 'fullColor',
  configuration,
});

describe('layout dosyaları', () => {
  it('layout klasörü boş değil', () => {
    assert.ok(FILES.length > 0);
  });

  it('sekiz widget dosyası', { skip: FILES.length < WIDGET_NAMES.length && 'kalan layout görevleri bekleniyor' }, () => {
    assert.equal(FILES.length, WIDGET_NAMES.length);
  });

  for (const file of FILES) {
    describe(file, () => {
      const source = read(file);
      const ast = parse(source, file);

      it('yalnız izinli modülleri import eder; diğerleri yalnız tip', () => {
        for (const node of ast.program.body) {
          if (node.type !== 'ImportDeclaration' || node.importKind === 'type') continue;
          assert.ok(ALLOWED_IMPORTS.includes(node.source.value), `değer import'u: ${node.source.value}`);
        }
      });

      it("modül düzeyinde yalnız 'widget' layout'u ve `create` var", () => {
        const layouts = [];
        for (const node of ast.program.body) {
          if (node.type === 'ImportDeclaration' || node.type === 'TSTypeAliasDeclaration') continue;
          const declaration = node.type === 'ExportNamedDeclaration' ? node.declaration : node;
          assert.equal(declaration?.type, 'VariableDeclaration', `beklenmeyen ifade: ${node.type}`);
          assert.equal(declaration.declarations.length, 1);
          const { id, init } = declaration.declarations[0];
          if (node.type === 'ExportNamedDeclaration') {
            assert.equal(id.name, 'create');
            continue;
          }
          layouts.push(init);
        }
        assert.equal(layouts.length, 1);
        const [fn] = layouts;
        assert.equal(fn.type, 'ArrowFunctionExpression');
        assert.equal(fn.body.directives[0]?.value.value, 'widget', "'widget' gövdenin ilk satırı");
      });

      it('createWidget adı WIDGET_NAMES içinde', () => {
        const matches = [...source.matchAll(/createWidget(?:<[^(]*>)?\(\s*'(\w+)'/g)];
        assert.equal(matches.length, 1);
        assert.ok((WIDGET_NAMES as readonly string[]).includes(matches[0][1]), matches[0][1]);
      });

      it('hook, async, ağ, renk sabiti yok', () => {
        assert.doesNotMatch(source, /\b(useState|useEffect|await|async|fetch)\b|https?:/);
        assert.doesNotMatch(source, /#[0-9a-fA-F]{3,8}\b/);
      });

      for (const isDev of [false, true]) {
        it(`babel-preset-expo gövdeyi metne çevirir (isDev=${isDev})`, () => {
          const { layout } = widgetOf(file, isDev);
          assert.equal(typeof layout, 'string');
          assert.match(layout, /^function\(/);
          assert.doesNotMatch(layout, /^function\([^)]*\)\{\s*['"]widget/);
        });
      }

      it("gövdedeki serbest adlar extension'da küresel", () => {
        const known = new Set([...expoUiNames(), ...RUNTIME_GLOBALS]);
        const { layout } = widgetOf(file, false);
        const tree = parse(`(${layout})`, 'layout.js');
        let globals: string[] = [];
        babel.traverse(tree, {
          Program(p: { scope: { globals: object } }) {
            globals = Object.keys(p.scope.globals);
          },
        });
        assert.ok(globals.length > 0);
        assert.deepEqual(globals.filter((name) => !known.has(name)), []);
      });

      it("`ok` dışı durumlar yalnız `card` ile her boyda çizilir", () => {
        const { layout } = widgetOf(file, false);
        const render = evaluate(layout);
        for (const state of NON_OK) {
          for (const rtl of [false, true]) {
            for (const family of HOME) {
              const card = { eyebrow: 'Başlık', title: 'Durum', body: 'Açıklama', cta: 'Giriş yap' };
              const root = render(stripNulls({ ...base(state, rtl), card }), env(family));
              assert.ok(collect(root).includes('Durum'), `${state} ${family}`);
              assert.ok(modifierTypes(root).includes('containerBackground'));
              assert.ok(modifierTypes(root).includes('widgetURL'));
              // Kartsız da düşmez.
              render(stripNulls(base(state, rtl)), env(family));
            }
          }
        }
      });

      it('kilit ekranı: `ok` dışı durumlarda yalnız işaret ve kısa metin; renk yok', () => {
        const { layout } = widgetOf(file, false);
        const render = evaluate(layout);
        const card = { eyebrow: 'Başlık', title: 'Kısa metin', body: 'Açıklama', cta: 'Giriş yap' };
        for (const state of NON_OK) {
          for (const rtl of [false, true]) {
            for (const family of LOCK) {
              const root = render(stripNulls({ ...base(state, rtl), card }), env(family));
              const texts = collect(root);
              assert.ok(texts.every((t) => t === 'file:///mark-mono.png' || t === 'Kısa metin'), `${state} ${family}: ${texts}`);
              assert.ok(texts.includes('file:///mark-mono.png'));
              assert.ok(modifierTypes(root).includes('containerBackground'));
              assert.ok(modifierTypes(root).includes('widgetURL'));
              assert.ok(flatten(root).every((n) => !modifierTypes(n).includes('foregroundStyle')));
              render(stripNulls(base(state, rtl)), env(family));
            }
          }
        }
      });

      it('kilit ekranı: veride ad, plaka, ürün, kapak, renk yok; satır içi tek `Image` + `Text`', () => {
        const { name, layout } = widgetOf(file, false);
        const fixture = FIXTURES[name];
        if (!fixture) return;
        const render = evaluate(layout);
        for (const rtl of [false, true]) {
          for (const family of LOCK) {
            const root = render(stripNulls(fixture(rtl)), env(family, 'dark'));
            const dump = JSON.stringify(root);
            for (const value of PRIVATE_VALUES) assert.ok(!dump.includes(value), `${name} ${family}: ${value}`);
            assert.ok(collect(root).includes('file:///mark-mono.png'));
            assert.ok(!dump.includes('file:///mark.png'));
            assert.ok(flatten(root).every((n) => !modifierTypes(n).includes('foregroundStyle')));
            assert.ok(modifierTypes(root).includes('containerBackground'));
            assert.ok(!modifierTypes(root).includes('background'));
            if (family === 'accessoryCircular') {
              assert.ok(flatten(root).some((n) => n.type === 'AccessoryWidgetBackground'));
            }
            if (family === 'accessoryInline') {
              assert.deepEqual(flatten(root).map((n) => n.type).slice(1).sort(), ['Image', 'Text']);
            }
          }
        }
      });

      it('veriyle her boy, tema ve yönde çizilir (`null` alanlar atılmış)', () => {
        const { name, layout } = widgetOf(file, false);
        const fixture = FIXTURES[name];
        if (!fixture) return;
        const render = evaluate(layout);
        for (const rtl of [false, true]) {
          for (const scheme of ['light', 'dark']) {
            for (const family of HOME) {
              const root = render(stripNulls(fixture(rtl)), env(family, scheme));
              assert.ok(modifierTypes(root).includes('containerBackground'), `${name} ${family}`);
              assert.ok(modifierTypes(root).includes('widgetURL'), `${name} ${family}`);
            }
          }
        }
      });
    });
  }
});

describe('Koruma düzeni', () => {
  const render = evaluate(widgetOf('koruma.widget.tsx', false).layout);

  it('kök adresi `props.url`', () => {
    const root = render(stripNulls(korumaProps(false)), env('systemSmall'));
    const url = (root.props.modifiers as Modifier[]).find((m) => m.$type === 'widgetURL');
    assert.deepEqual(url?.args, ['olexfilms://home']);
  });

  it('araç seçimi: bulunan, yoksa ilk; `auto` ilk', () => {
    const texts = (vehicle?: string) => {
      const props = stripNulls(korumaProps(false));
      return collect(render(props, env('systemSmall', 'light', vehicle ? { vehicle } : undefined)));
    };
    assert.ok(texts().some((t) => t.includes('Olex PPF Parlak')));
    assert.ok(texts('auto').some((t) => t.includes('Olex PPF Parlak')));
    assert.ok(texts('silinmis').some((t) => t.includes('Olex PPF Parlak')));
    assert.ok(texts('b').some((t) => t.includes('Tek Film')));
    assert.ok(texts('c').includes('Aktif garanti yok'));
  });

  it('büyük boy üç garanti, plaka rozeti ve alt satır', () => {
    const texts = collect(render(stripNulls(korumaProps(false)), env('systemLarge')));
    for (const expected of ['Porsche 911', '3 aktif garanti', 'TR', '34 ABC 123', 'Olex Seramik', 'Olex Cam Filmi', 'Son hizmet 12 Eki 2026', 'Güncellendi 14:05']) {
      assert.ok(texts.includes(expected), expected);
    }
  });

  it('orta boy iki garanti, tek garantiyle de çizilir', () => {
    const two = collect(render(stripNulls(korumaProps(false)), env('systemMedium')));
    assert.ok(two.includes('Olex Seramik'));
    assert.ok(!two.includes('Olex Cam Filmi'));
    const one = collect(render(stripNulls(korumaProps(false)), env('systemMedium', 'light', { vehicle: 'b' })));
    assert.ok(one.includes('Tek Film'));
  });

  it('kapak yoksa çizilmez; aynalanmaz, Arapçada karşı kenara geçer', () => {
    const props = stripNulls(korumaProps(false));
    props.vehicles[0].heroUri = null;
    assert.ok(!collect(render(props, env('systemMedium'))).includes('file:///hero.png'));
    const heroOf = (rtl: boolean) =>
      flatten(render(stripNulls(korumaProps(rtl)), env('systemMedium'))).find((n) => n.type === 'Image' && n.props.uiImage === 'file:///hero.png');
    const shift = (rtl: boolean) => (heroOf(rtl)?.props.modifiers as Modifier[]).find((m) => m.$type === 'offset')?.args[0];
    assert.deepEqual(shift(false), { x: 14, y: -4 });
    assert.deepEqual(shift(true), { x: -22, y: -6 });
    assert.ok(!modifierTypes(heroOf(true) as Node).includes('scaleEffect'));
  });

  it('bayat veri kartı: süre ve `staleLabel`', () => {
    const props = { ...stripNulls(korumaProps(false)), staleLabel: 'Güncellenemedi. Son veri 2 saat önce.' };
    const texts = collect(render(props, env('systemSmall')));
    assert.ok(texts.includes('2 yıl 4 ay kaldı'));
    assert.ok(texts.includes('Güncellenemedi. Son veri 2 saat önce.'));
  });

  it('Arapçada yatay satır ters, plaka rozeti değil', () => {
    const header = (rtl: boolean) => {
      const root = render(stripNulls(korumaProps(rtl)), env('systemLarge'));
      const flat = root.props.children.flat(Infinity);
      return flat[0].props.children.filter(Boolean);
    };
    assert.equal(header(false)[0].type, 'Image');
    const reversed = header(true);
    assert.equal(reversed[reversed.length - 1].type, 'Image');
    const badge = reversed[0];
    const [strip, plate] = badge.props.children;
    assert.equal(strip.props.children, 'TR');
    assert.equal(plate.props.children, '34 ABC 123');
  });

  it('kök işaret: ana ekranda renkli, tek renkli modda beyaz', () => {
    const props = stripNulls(korumaProps(false));
    assert.ok(collect(render(props, env('systemSmall'))).includes('file:///mark.png'));
    const tinted = { ...env('systemSmall'), widgetRenderingMode: 'accented' };
    assert.ok(collect(render(props, tinted)).includes('file:///mark-mono.png'));
  });
});

describe('kilit ekranı metinleri', () => {
  const lockTexts = (file: string, props: object, family: string) =>
    collect(evaluate(widgetOf(file, false).layout)(stripNulls(props), env(family))).filter((t) => !t.startsWith('file://'));

  it('Koruma', () => {
    const props = korumaProps(false);
    assert.deepEqual(lockTexts('koruma.widget.tsx', props, 'accessoryCircular'), ['2y 4a']);
    assert.deepEqual(lockTexts('koruma.widget.tsx', props, 'accessoryRectangular'), ['Koruman', '2 yıl 4 ay kaldı']);
    assert.deepEqual(lockTexts('koruma.widget.tsx', props, 'accessoryInline'), ['Garanti 2 yıl 4 ay']);
  });

  it('Koruma, seçilen araç ve garantisiz araç', () => {
    const render = evaluate(widgetOf('koruma.widget.tsx', false).layout);
    const props = stripNulls(korumaProps(false));
    const circular = render(props, env('accessoryCircular', 'light', { vehicle: 'c' }));
    assert.deepEqual(collect(circular), ['file:///mark-mono.png']);
    const rectangular = render(props, env('accessoryRectangular', 'light', { vehicle: 'c' }));
    assert.ok(collect(rectangular).includes('Aktif garanti yok'));
  });

  it('Panom', () => {
    const props = panomProps(false);
    assert.deepEqual(lockTexts('panom.widget.tsx', props, 'accessoryCircular'), ['4', 'sırada']);
    assert.deepEqual(lockTexts('panom.widget.tsx', props, 'accessoryRectangular'), ['Panom', '4 sırada', '5 işlemde · 2 bitti']);
    assert.deepEqual(lockTexts('panom.widget.tsx', props, 'accessoryInline'), ['Panom · 4 sırada, 5 işlemde']);
  });
});

describe('kayıt noktası', () => {
  const ios = fs.readFileSync(path.join(DIR, 'index.ios.ts'), 'utf8');
  const fallback = fs.readFileSync(path.join(DIR, 'index.ts'), 'utf8');

  it('index.ts layout dosyası ve native modül yüklemez', () => {
    assert.doesNotMatch(fallback, /\.widget|require\(/);
    assert.doesNotMatch(fallback, /^import (?!type).*expo-widgets/m);
  });

  it('index.ios.ts layout dosyalarını yalnız tembel `require` ile, native modül kontrolünden sonra yükler', () => {
    assert.doesNotMatch(ios, /^import .*\.widget/m);
    const loader = ios.indexOf('function load()');
    assert.ok(ios.indexOf("requireOptionalNativeModule('ExpoWidgets')", loader) > loader);
    assert.ok(ios.indexOf('layout().create', loader) > ios.indexOf("requireOptionalNativeModule('ExpoWidgets')", loader));
  });

  it('her layout dosyası kayıtlı, kayıtlı her dosya var', () => {
    const registered = [...ios.matchAll(/require\('\.\/([\w.-]+)'\)/g)].map((m) => `${m[1]}.tsx`).sort();
    assert.deepEqual(registered, [...FILES].sort());
  });

  it('kayıttaki ad dosyanın widget adıyla aynı', () => {
    for (const file of FILES) {
      const { name } = widgetOf(file, false);
      assert.ok(ios.includes(`${name}: () => require('./${file.replace('.tsx', '')}')`), name);
    }
  });

  it("`index.ts` ile aynı dışa aktarımlar", () => {
    const exportsOf = (s: string) => [...s.matchAll(/^export (?:type |const |function )(\w+)/gm)].map((m) => m[1]).sort();
    const iosExports = [...exportsOf(ios), ...[...ios.matchAll(/^export type \{ ([^}]+) \}/gm)].flatMap((m) => m[1].split(', ').map((s) => s.trim()))].sort();
    const fallbackExports = [...fallback.matchAll(/^export (?:interface |type |const |function )(\w+)/gm)].map((m) => m[1]).sort();
    assert.deepEqual(iosExports, fallbackExports);
  });
});
