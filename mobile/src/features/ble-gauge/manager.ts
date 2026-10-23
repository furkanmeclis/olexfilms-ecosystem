/**
 * The single BLE manager of the app, for SmartPTG / NexPTG paint gauges.
 *
 * - Created lazily on first use; when the native module is missing (Expo Go,
 *   web, an old dev build) `isGaugeAvailable()` is false and nothing loads.
 * - Scans, connects, subscribes to RX notifications, decodes frames and turns
 *   them into readings with the serial's factory curve (cached on the phone).
 * - Remembers the last gauge per user for one-tap reconnect.
 */
import type { BleManager, Device, Subscription } from '@sfourdrinier/react-native-ble-plx';
import { PermissionsAndroid, Platform, TurboModuleRegistry } from 'react-native';

import { api } from '@/core/api/client';
import { readJSON, writeJSON } from '@/core/storage';

import { BUNDLED_CURVES } from './bundled-curves';
import { deviceModelName, frameToReading, type GaugeCurve, resolveCurve } from './curve';
import { adapterErrorKey, gaugeErrorKey } from './errors';
import { base64ToBinary, createLineSplitter, decodeFrame, GAUGE_BLE, isGaugePeripheral } from './protocol';
import { type Commit, initialSampler, type SamplerState, samplerProgress, stepSampler } from './readings';
import { createGeneration, createSerialConfirm, poweredOnTimeoutError, poweredOnWaitMs } from './session';
import { type GaugeState, initialGaugeState, upsertDevice, useGauge } from './store';

let available: boolean | null = null;

/** True on a build that ships the BLE native module (development/production build on a phone). */
export function isGaugeAvailable(): boolean {
  if (available === null) {
    try {
      available = (Platform.OS === 'ios' || Platform.OS === 'android') && TurboModuleRegistry.get('BlePlx') != null;
    } catch {
      available = false;
    }
  }
  return available;
}

let manager: BleManager | null = null;

function getManager(): BleManager {
  if (!isGaugeAvailable()) throw new Error('unsupported');
  if (!manager) {
    // Required lazily: the module resolves its TurboModule at import time.
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { BleManager: Manager } = require('@sfourdrinier/react-native-ble-plx') as typeof import('@sfourdrinier/react-native-ble-plx');
    manager = new Manager();
  }
  return manager;
}

const set = useGauge.setState;

/* ---------------------------------------------------------------- permissions */

let permissionPrompt = false;

/** True while an Android permission dialog is open (it sends the app to `background`). */
export function isPermissionPromptOpen(): boolean {
  return permissionPrompt;
}

/** Android runtime permissions; iOS asks on its own when the manager starts. */
async function ensurePermissions(): Promise<void> {
  if (Platform.OS !== 'android') return;
  const sdk = typeof Platform.Version === 'number' ? Platform.Version : Number(Platform.Version);
  const wanted =
    sdk >= 31
      ? [PermissionsAndroid.PERMISSIONS.BLUETOOTH_SCAN, PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT]
      : [PermissionsAndroid.PERMISSIONS.ACCESS_FINE_LOCATION];
  if ((await Promise.all(wanted.map((p) => PermissionsAndroid.check(p)))).every(Boolean)) return;
  permissionPrompt = true;
  let result: Awaited<ReturnType<typeof PermissionsAndroid.requestMultiple>>;
  try {
    result = await PermissionsAndroid.requestMultiple(wanted);
  } finally {
    // The AppState events of the dialog arrive right after it closes.
    setTimeout(() => {
      permissionPrompt = false;
    }, 1000);
  }
  if (!wanted.every((p) => result[p] === PermissionsAndroid.RESULTS.GRANTED)) throw new Error('unauthorized');
}

/**
 * Waits for the adapter to report PoweredOn. It starts as Unknown, and on iOS
 * stays Unknown while the first Bluetooth permission prompt is open, so the
 * wait is long in that state and short once the adapter has answered.
 */
function waitPoweredOn(ble: BleManager): Promise<void> {
  return new Promise((resolve, reject) => {
    let sub: Subscription | null = null;
    let last: string | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const done = () => {
      if (timer) clearTimeout(timer);
      sub?.remove();
    };
    const arm = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        done();
        reject(new Error(poweredOnTimeoutError(last)));
      }, poweredOnWaitMs(last));
    };
    arm();
    sub = ble.onStateChange((state) => {
      last = state;
      if (state === 'PoweredOn') {
        done();
        resolve();
        return;
      }
      const key = adapterErrorKey(state);
      if (key) {
        done();
        reject(new Error(key.replace('ble.error.', '')));
        return;
      }
      arm(); // Unknown / Resetting: keep waiting.
    }, true);
  });
}

async function ready(): Promise<BleManager> {
  await ensurePermissions();
  const ble = getManager();
  await waitPoweredOn(ble);
  return ble;
}

/* ----------------------------------------------------------------------- scan */

let scanTimer: ReturnType<typeof setTimeout> | null = null;

export function stopScan() {
  if (scanTimer) clearTimeout(scanTimer);
  scanTimer = null;
  if (manager) void manager.stopDeviceScan().catch(() => undefined);
  if (useGauge.getState().status === 'scanning') set({ status: 'idle' });
}

/**
 * Lists nearby gauges for `GAUGE_BLE.scanTimeoutMs`. The NexPTG app finds the
 * gauge by its advertised name; the service UUID is not always in the
 * advertisement, so the scan is unfiltered and isGaugePeripheral() accepts a
 * peripheral by name or by the advertised NUS service.
 */
export async function startScan(): Promise<void> {
  if (useGauge.getState().status === 'connecting') return;
  stopScan();
  set({ error: null, devices: [] });
  try {
    const ble = await ready();
    set({ status: 'scanning' });
    await ble.startDeviceScan(null, { allowDuplicates: false }, (error, d) => {
      if (error) {
        set({ error: gaugeErrorKey(error) });
        stopScan();
        return;
      }
      if (!d || !isGaugePeripheral(d.name ?? d.localName, d.serviceUUIDs)) return;
      set((s) => ({
        devices: upsertDevice(s.devices, { id: d.id, name: d.name ?? d.localName ?? 'SmartPTG', rssi: d.rssi }),
      }));
    });
    scanTimer = setTimeout(stopScan, GAUGE_BLE.scanTimeoutMs);
  } catch (e) {
    set({ status: 'idle', error: gaugeErrorKey(e) });
  }
}

/* ---------------------------------------------------------------------- curve */

const curveKey = (serial: number) => `ble.gauge.curve.${serial}`;
let curve: GaugeCurve | null = null;
let curveLoading: number | null = null;

async function loadCurve(serial: number) {
  if (curveLoading === serial) return;
  if (curve?.serial === serial) {
    set({ curve: 'ready', model: deviceModelName(curve.deviceType) });
    return;
  }
  const cached = readJSON<GaugeCurve>(curveKey(serial));
  if (cached?.serial === serial) {
    curve = cached;
    set({ curve: 'ready', model: deviceModelName(cached.deviceType) });
    return;
  }
  curveLoading = serial;
  set({ curve: 'loading' });
  try {
    const { curve: parsed } = await resolveCurve(serial, fetchBackendCurve, BUNDLED_CURVES);
    writeJSON(curveKey(serial), parsed);
    curve = parsed;
    set({ curve: 'ready', model: deviceModelName(parsed.deviceType) });
  } catch {
    set({ curve: 'error', error: 'ble.error.curve' });
  } finally {
    curveLoading = null;
  }
}

/**
 * Raw `.dat` of a serial from our backend (TEC-934), through the mobile
 * mirror with the session's bearer. Null on 404; throws on anything else.
 */
async function fetchBackendCurve(serial: number): Promise<string | null> {
  const abort = new AbortController();
  const timer = setTimeout(() => abort.abort(), 15_000);
  try {
    const { data, response } = await api.GET('/v1/measurements/devices/smartptg/curves/{serial}', {
      params: { path: { serial: String(serial) } },
      parseAs: 'text',
      signal: abort.signal,
    });
    if (response.status === 404) return null;
    if (!response.ok || typeof data !== 'string') throw new Error(`curve_http_${response.status}`);
    return data;
  } finally {
    clearTimeout(timer);
  }
}

/** Retries a failed curve download (the gauge stays connected). */
export function retryCurve() {
  const serial = useGauge.getState().serial;
  if (serial != null) {
    set({ error: null });
    void loadCurve(serial);
  }
}

/* ------------------------------------------------------------ reading stream */

type CommitListener = (c: Commit) => void;
const commitListeners = new Set<CommitListener>();
let sampler: SamplerState = initialSampler;

/** Steady readings, one per probe contact (see readings.ts stepSampler). */
export function subscribeCommits(listener: CommitListener): () => void {
  commitListeners.add(listener);
  return () => {
    commitListeners.delete(listener);
  };
}

const splitter = createLineSplitter();
const serialConfirm = createSerialConfirm(2);

function onLine(line: string) {
  const frame = decodeFrame(line);
  if (!frame) return;
  // A serial counts (and may start a curve download) only after two frames agree.
  const serial = serialConfirm.push(frame.serial);
  if (serial != null && useGauge.getState().serial !== serial) {
    set({ serial });
    void loadCurve(serial);
  }
  const reading = frameToReading(frame, curve, null, Date.now());
  const step = stepSampler(sampler, reading);
  sampler = step.state;
  set({ reading, battery: frame.batteryVoltage, progress: samplerProgress(sampler) });
  if (step.commit) for (const l of commitListeners) l(step.commit);
}

/* ----------------------------------------------------------------- connection */

let device: Device | null = null;
let subs: Subscription[] = [];
let userDisconnect = false;
/** Connect attempts; disconnect() bumps it so late connectToDevice results are dropped. */
const generation = createGeneration();
/** Peripheral of the connect attempt in flight, cancelled by disconnect(). */
let pendingId: string | null = null;

const lastKey = (userKey: string) => `ble.gauge.last.${userKey}`;

export function lastGauge(userKey: string | null | undefined): { id: string; name: string } | null {
  return userKey ? readJSON<{ id: string; name: string }>(lastKey(userKey)) : null;
}

function teardown() {
  for (const s of subs) s.remove();
  subs = [];
  splitter.reset();
  serialConfirm.reset();
  sampler = initialSampler;
  device = null;
}

function cancelPeripheral(id: string) {
  if (manager) void manager.cancelDeviceConnection(id).catch(() => undefined);
}

/** Ends the session in the error state (kept until the user retries or dismisses it). */
function fail(error: GaugeState['error']) {
  const id = device?.id;
  teardown();
  set({ ...initialGaugeState, devices: useGauge.getState().devices, status: 'error', error });
  if (id) cancelPeripheral(id);
}

export async function connect(id: string, userKey?: string | null): Promise<void> {
  stopScan();
  if (device || pendingId) await disconnect();
  const ticket = generation.next();
  const stale = () => !generation.isCurrent(ticket);
  userDisconnect = false;
  pendingId = id;
  set({ status: 'connecting', error: null });
  try {
    const ble = await ready();
    if (stale()) return;
    const d = await ble.connectToDevice(id, { timeout: 12_000 });
    if (stale()) {
      // The screen closed or the app left while connecting: do not keep the link.
      cancelPeripheral(id);
      return;
    }
    await d.discoverAllServicesAndCharacteristics();
    const chars = await d.characteristicsForService(GAUGE_BLE.serviceUUID).catch(() => []);
    if (stale()) {
      cancelPeripheral(id);
      return;
    }
    if (!chars.some((c) => c.uuid.toLowerCase() === GAUGE_BLE.rxCharacteristicUUID)) throw new Error('service');
    pendingId = null;
    device = d;
    const name = d.name ?? d.localName ?? 'SmartPTG';
    subs.push(
      ble.onDeviceDisconnected(d.id, () => {
        if (userDisconnect) return;
        teardown();
        set({ ...initialGaugeState, devices: useGauge.getState().devices, status: 'error', error: 'ble.error.disconnected' });
      }),
    );
    subs.push(
      d.monitorCharacteristicForService(GAUGE_BLE.serviceUUID, GAUGE_BLE.rxCharacteristicUUID, (error, ch) => {
        if (error) {
          if (stale() || userDisconnect || device !== d) return;
          const key = gaugeErrorKey(error);
          fail(key === 'ble.error.generic' ? 'ble.error.stream' : key);
          return;
        }
        if (ch?.value) for (const line of splitter.push(base64ToBinary(ch.value))) onLine(line);
      }),
    );
    if (userKey) writeJSON(lastKey(userKey), { id: d.id, name });
    set({ status: 'connected', device: { id: d.id, name } });
  } catch (e) {
    if (stale()) return;
    pendingId = null;
    teardown();
    set({ status: 'error', device: null, error: gaugeErrorKey(e) });
    cancelPeripheral(id);
  }
}

/** Reconnects to the user's last gauge: directly, then via a short scan for the same id. */
export async function reconnectLast(userKey: string | null | undefined): Promise<void> {
  const last = lastGauge(userKey);
  const status = useGauge.getState().status;
  if (!last || (status !== 'idle' && status !== 'error')) return;
  await connect(last.id, userKey);
  if (useGauge.getState().status === 'connected') return;

  await startScan();
  const seen = (s: GaugeState) => s.devices.some((d) => d.id === last.id);
  const found = await new Promise<boolean>((resolve) => {
    const now = useGauge.getState();
    if (seen(now) || now.status !== 'scanning') {
      resolve(seen(now));
      return;
    }
    const unsub = useGauge.subscribe((s) => {
      if (seen(s) || s.status !== 'scanning') {
        unsub();
        resolve(seen(s));
      }
    });
  });
  if (found) await connect(last.id, userKey);
  else if (!useGauge.getState().error) set({ error: 'ble.error.not_found' });
}

export async function disconnect(): Promise<void> {
  userDisconnect = true;
  generation.bump();
  const ids = new Set([device?.id ?? useGauge.getState().device?.id, pendingId].filter((x): x is string => !!x));
  pendingId = null;
  teardown();
  set({ ...initialGaugeState, devices: useGauge.getState().devices });
  const ble = manager;
  if (ble) await Promise.all([...ids].map((id) => ble.cancelDeviceConnection(id).catch(() => undefined)));
}

export function clearError() {
  set((s) => ({ error: null, status: s.status === 'error' ? 'idle' : s.status }));
}

/** Stops everything (screen left or app sent to the background). */
export async function shutdown(): Promise<void> {
  stopScan();
  await disconnect();
}
