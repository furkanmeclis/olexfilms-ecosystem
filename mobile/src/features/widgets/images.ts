// Widget görsellerini App Group alanına yazar. Dosya işlemleri native; sınama cihazda (Task 24).
import { Asset } from 'expo-asset';
import { Directory, File, Paths } from 'expo-file-system';
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';
import { widgetsDirectory } from 'expo-widgets';

import type { components } from '@/core/api/schema';
import { brandLogoUrl, heroUrl } from '@/features/customer-space/hero-url';

import { HERO_MAX_WIDTH_PX, MARK_FILE, MARK_MONO_FILE } from './config';
import { heroFileName, isFresh, logoFileName, qrFileName, staleFiles } from './image-names';
import { qrPngFromText } from './qr-png';

type PortalVehicle = components['schemas']['PortalVehicle'];

const LOGO_MAX_WIDTH_PX = 160;

/** Paylaşılan alan; entitlement yoksa veya Android'de `null`, tüm işlem atlanır. */
function sharedDir(): Directory | null {
  // Native sabit tipte `string`, ama App Group yoksa `null` döner.
  const uri = widgetsDirectory as string | null | undefined;
  return uri ? new Directory(uri) : null;
}

function downloadDir(): Directory {
  const dir = new Directory(Paths.cache, 'widget-downloads');
  if (!dir.exists) dir.create({ intermediates: true, idempotent: true });
  return dir;
}

/** SVG saklanmaz (ImageView çizemez); `.src` uzantısı MIME vermez, bu yüzden ilk baytlara bakılır. */
function isSvg(file: File): boolean {
  if (file.type === 'image/svg+xml') return true;
  const handle = file.open();
  try {
    const head = new TextDecoder().decode(handle.readBytesSync(256));
    return /<svg|<\?xml/i.test(head);
  } finally {
    handle.close();
  }
}

function remove(file: File) {
  try {
    if (file.exists) file.delete();
  } catch {
    // Silinemeyen geçici dosya bir sonraki eşitlemede ezilir.
  }
}

/**
 * Uzak görseli indirir, `width` px'e küçültür, PNG olarak (alfa korunur) paylaşılan alana yazar.
 * Taze dosya varsa ağa çıkmaz. Her hatada `null`: widget görselsiz çizilir.
 */
async function storeRemote(url: string, fileName: string, width: number): Promise<string | null> {
  const shared = sharedDir();
  if (!shared) return null;
  const target = new File(shared, fileName);
  if (target.exists && isFresh(target.lastModified, Date.now())) return target.uri;

  const source = new File(downloadDir(), `${fileName}.src`);
  let saved: File | null = null;
  try {
    await File.downloadFileAsync(url, source, { idempotent: true });
    if (isSvg(source)) return null;
    const ref = await ImageManipulator.manipulate(source.uri, { maxWidth: width })
      .resize({ width, height: null })
      .renderAsync();
    const result = await ref.saveAsync({ format: SaveFormat.PNG });
    saved = new File(result.uri);
    await saved.copy(target, { overwrite: true });
    return target.uri;
  } catch {
    return null;
  } finally {
    remove(source);
    if (saved) remove(saved);
  }
}

export function storeHero(v: Pick<PortalVehicle, 'car_brand' | 'car_model'>): Promise<string | null> {
  const name = heroFileName(v);
  return name ? storeRemote(heroUrl(v), name, HERO_MAX_WIDTH_PX) : Promise.resolve(null);
}

export function storeBrandLogo(brandUuid: string): Promise<string | null> {
  const name = logoFileName(brandUuid);
  return name ? storeRemote(brandLogoUrl(brandUuid), name, LOGO_MAX_WIDTH_PX) : Promise.resolve(null);
}

/** `text` `publicWarrantyUrl(code)` sonucudur; QR cihazda üretilir, ağ yok. */
export function storeQr(code: string, text: string): string | null {
  const shared = sharedDir();
  const name = qrFileName(code);
  if (!shared || !name) return null;
  try {
    const target = new File(shared, name);
    target.write(qrPngFromText(text));
    return target.uri;
  } catch {
    return null;
  }
}

const MARK_ASSETS = [
  { file: MARK_FILE, module: require('../../../assets/images/widget-mark.png') as number },
  { file: MARK_MONO_FILE, module: require('../../../assets/images/widget-mark-mono.png') as number },
];

/** Altın işaret ve beyaz tek renkli sürüm; her eşitlemede üzerine yazılır (uygulama güncellemesi gelebilir). */
export async function storeMarks(): Promise<{ markUri: string | null; markMonoUri: string | null }> {
  const shared = sharedDir();
  if (!shared) return { markUri: null, markMonoUri: null };
  const uris: (string | null)[] = [];
  for (const { file, module } of MARK_ASSETS) {
    try {
      const asset = await Asset.fromModule(module).downloadAsync();
      if (!asset.localUri) throw new Error('no local uri');
      const target = new File(shared, file);
      await new File(asset.localUri).copy(target, { overwrite: true });
      uris.push(target.uri);
    } catch {
      uris.push(null);
    }
  }
  return { markUri: uris[0], markMonoUri: uris[1] };
}

function listFiles(shared: Directory): File[] {
  return shared.list().filter((entry): entry is File => entry instanceof File);
}

function removeAll(files: File[], doomed: string[]) {
  const names = new Set(doomed);
  for (const file of files) if (names.has(file.name)) remove(file);
}

/** `keep` (dosya adları) dışındakileri siler; Olex işaretleri kalır. */
export function reconcile(keep: Set<string>) {
  const shared = sharedDir();
  if (!shared) return;
  try {
    const files = listFiles(shared);
    removeAll(files, staleFiles(files.map((f) => f.name), keep));
  } catch {
    // Dizin okunamadı: bir sonraki eşitlemede yeniden denenir.
  }
}

/** Çıkışta: işaretler dışında her şeyi siler. */
export function purgeAll() {
  reconcile(new Set());
}
