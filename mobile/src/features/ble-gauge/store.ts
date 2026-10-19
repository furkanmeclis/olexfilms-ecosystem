import { create } from 'zustand';

import type { GaugeReading } from './curve';
import type { GaugeErrorKey } from './errors';

export type GaugeStatus = 'idle' | 'scanning' | 'connecting' | 'connected';

export type ScannedGauge = { id: string; name: string; rssi: number | null };

export type GaugeState = {
  status: GaugeStatus;
  /** Gauges found by the current/last scan, strongest signal first. */
  devices: ScannedGauge[];
  device: { id: string; name: string } | null;
  /** Serial, model and battery come with the first frame / the curve. */
  serial: number | null;
  model: string | null;
  battery: number | null;
  curve: 'none' | 'loading' | 'ready' | 'error';
  reading: GaugeReading | null;
  /** 0…1 progress of the current probe contact. */
  progress: number;
  error: GaugeErrorKey | null;
};

export const initialGaugeState: GaugeState = {
  status: 'idle',
  devices: [],
  device: null,
  serial: null,
  model: null,
  battery: null,
  curve: 'none',
  reading: null,
  progress: 0,
  error: null,
};

/** Connection and live reading state of the BLE gauge; written only by manager.ts. */
export const useGauge = create<GaugeState>(() => initialGaugeState);

export function upsertDevice(list: ScannedGauge[], d: ScannedGauge): ScannedGauge[] {
  const next = list.some((x) => x.id === d.id) ? list.map((x) => (x.id === d.id ? d : x)) : [...list, d];
  return next.sort((a, b) => (b.rssi ?? -999) - (a.rssi ?? -999));
}
