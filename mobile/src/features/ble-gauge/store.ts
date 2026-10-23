import { create } from 'zustand';

import type { StoredCalibration } from './calibration';
import type { GaugeReading, Material } from './curve';
import type { GaugeErrorKey } from './errors';

export type GaugeStatus = 'idle' | 'scanning' | 'connecting' | 'connected' | 'error';

export type ScannedGauge = { id: string; name: string; rssi: number | null };

/** The calibration wizard's live state (manager.ts startCalibrationStep). */
export type CalibrationRun = {
  phase: 'idle' | 'measuring' | 'error';
  material: Material | null;
  /** 0…1: steady frames of the running step. */
  progress: number;
  error: 'unstable' | 'timeout' | 'not_ready' | null;
  /** Offsets found so far in this run (Hz), committed by finishCalibration. */
  results: Partial<Record<Material, { offset: number; samples: number[] }>>;
};

export const idleCalibration: CalibrationRun = { phase: 'idle', material: null, progress: 0, error: null, results: {} };

export type GaugeState = {
  status: GaugeStatus;
  /** Gauges found by the current/last scan, strongest signal first. */
  devices: ScannedGauge[];
  device: { id: string; name: string } | null;
  /** Serial, model and battery come with the first frame / the curve. */
  serial: number | null;
  model: string | null;
  /** Curve device type (NexPTGConfig.DeviceType), null until the curve is loaded. */
  deviceType: number | null;
  battery: number | null;
  /** Battery percent of the NexPTG app's table (protocol.ts batteryPercent). */
  batteryPercent: number | null;
  curve: 'none' | 'loading' | 'ready' | 'error';
  /** Backend ETag of the curve file (hex SHA-256 of the raw file), null for a bundled curve. */
  curveSha256: string | null;
  /** Foil thickness of the loaded curve (90 / 150 µm), null without a curve. */
  calibrationTargetUm: 90 | 150 | null;
  reading: GaugeReading | null;
  /** 0…1 progress of the current probe contact. */
  progress: number;
  /** No frame for GAUGE_BLE.noDataTimeoutMs while connected. */
  noData: boolean;
  /** Current calibration of the connected serial (local or from the backend). */
  calibration: StoredCalibration | null;
  calibrationRun: CalibrationRun;
  error: GaugeErrorKey | null;
};

export const initialGaugeState: GaugeState = {
  status: 'idle',
  devices: [],
  device: null,
  serial: null,
  model: null,
  deviceType: null,
  battery: null,
  batteryPercent: null,
  curve: 'none',
  curveSha256: null,
  calibrationTargetUm: null,
  reading: null,
  progress: 0,
  noData: false,
  calibration: null,
  calibrationRun: idleCalibration,
  error: null,
};

/** Connection and live reading state of the BLE gauge; written only by manager.ts. */
export const useGauge = create<GaugeState>(() => initialGaugeState);

export function upsertDevice(list: ScannedGauge[], d: ScannedGauge): ScannedGauge[] {
  const next = list.some((x) => x.id === d.id) ? list.map((x) => (x.id === d.id ? d : x)) : [...list, d];
  return next.sort((a, b) => (b.rssi ?? -999) - (a.rssi ?? -999));
}
