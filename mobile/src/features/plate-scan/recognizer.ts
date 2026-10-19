import { NativeModule, requireOptionalNativeModule } from 'expo';

import type { NormRect, OcrLine } from './lib';

/**
 * On-device plate recognition engine. Not part of the core app: the native
 * module only exists in dev/store builds that include `modules/plate-ocr`,
 * and on Android the text model is downloaded by Google Play services on
 * demand. Every caller goes through this interface, so another engine (a
 * downloaded detector model, a different OCR) can be swapped in later.
 */
export type RecognizerStatus =
  /** Ready to recognize. */
  | 'ready'
  /** Engine present, model not downloaded yet (Android). */
  | 'not_installed'
  /** No engine in this build (Expo Go, web) or the device cannot run it. */
  | 'unsupported';

export type OcrResult = { width: number; height: number; lines: OcrLine[] };

export interface PlateRecognizer {
  readonly id: string;
  /** Native module compiled into this build. */
  readonly supported: boolean;
  getStatus(): Promise<RecognizerStatus>;
  /** Downloads/installs what the engine needs; `onProgress` gets 0–1. */
  prepare(onProgress?: (progress: number) => void): Promise<void>;
  /** Frees the downloaded model again (no-op where the OS owns it). */
  remove(): Promise<void>;
  /** Text lines of a photo file (`file://…`). */
  recognize(uri: string): Promise<OcrResult>;
}

type NativeLine = { text: string; confidence?: number | null; alternatives?: string[] | null; box?: NormRect | null };

declare class PlateOcrNative extends NativeModule<{ onPrepareProgress: (e: { progress: number }) => void }> {
  getStatus(): Promise<RecognizerStatus>;
  prepare(): Promise<RecognizerStatus>;
  release(): Promise<boolean>;
  recognize(uri: string): Promise<{ width: number; height: number; lines: NativeLine[] }>;
}

const native = requireOptionalNativeModule<PlateOcrNative>('PlateOcr');

const nativeRecognizer: PlateRecognizer = {
  id: 'native-ocr',
  supported: native != null,
  async getStatus() {
    if (!native) return 'unsupported';
    try {
      return await native.getStatus();
    } catch {
      return 'unsupported';
    }
  },
  async prepare(onProgress) {
    if (!native) throw new Error('PlateOcr native module is not in this build');
    const sub = onProgress ? native.addListener('onPrepareProgress', (e) => onProgress(e.progress)) : null;
    try {
      await native.prepare();
      onProgress?.(1);
    } finally {
      sub?.remove();
    }
  },
  async remove() {
    if (native) await native.release();
  },
  async recognize(uri) {
    if (!native) throw new Error('PlateOcr native module is not in this build');
    const r = await native.recognize(uri);
    return {
      width: r.width,
      height: r.height,
      lines: r.lines.map((l) => ({ text: l.text, confidence: l.confidence ?? null, alternatives: l.alternatives ?? null, box: l.box ?? null })),
    };
  },
};

export function getPlateRecognizer(): PlateRecognizer {
  return nativeRecognizer;
}
