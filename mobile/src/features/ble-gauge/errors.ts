/**
 * BLE failures → i18n keys of the `measure` namespace (pure, alias-free).
 * Codes are react-native-ble-plx BleErrorCode values; the manager's own
 * failures are Errors whose message is one of the names below.
 */
export type GaugeErrorKey =
  | 'ble.error.unsupported'
  | 'ble.error.unauthorized'
  | 'ble.error.powered_off'
  | 'ble.error.location_off'
  | 'ble.error.not_found'
  | 'ble.error.connect'
  | 'ble.error.disconnected'
  | 'ble.error.service'
  | 'ble.error.timeout'
  | 'ble.error.curve'
  | 'ble.error.generic';

const BY_CODE: Record<number, GaugeErrorKey> = {
  3: 'ble.error.timeout', // OperationTimedOut
  100: 'ble.error.unsupported', // BluetoothUnsupported
  101: 'ble.error.unauthorized', // BluetoothUnauthorized
  102: 'ble.error.powered_off', // BluetoothPoweredOff
  103: 'ble.error.powered_off', // BluetoothInUnknownState
  104: 'ble.error.powered_off', // BluetoothResetting
  200: 'ble.error.connect', // DeviceConnectionFailed
  201: 'ble.error.disconnected', // DeviceDisconnected
  204: 'ble.error.not_found', // DeviceNotFound
  205: 'ble.error.disconnected', // DeviceNotConnected
  300: 'ble.error.service', // ServicesDiscoveryFailed
  302: 'ble.error.service', // ServiceNotFound
  400: 'ble.error.service', // CharacteristicsDiscoveryFailed
  403: 'ble.error.service', // CharacteristicNotifyChangeFailed
  404: 'ble.error.service', // CharacteristicNotFound
  601: 'ble.error.location_off', // LocationServicesDisabled
};

const BY_NAME: Record<string, GaugeErrorKey> = {
  unsupported: 'ble.error.unsupported',
  unauthorized: 'ble.error.unauthorized',
  powered_off: 'ble.error.powered_off',
  not_found: 'ble.error.not_found',
  timeout: 'ble.error.timeout',
  curve: 'ble.error.curve',
};

/** Any thrown value → message key. */
export function gaugeErrorKey(error: unknown): GaugeErrorKey {
  if (error && typeof error === 'object') {
    const code = (error as { errorCode?: unknown }).errorCode;
    if (typeof code === 'number' && BY_CODE[code]) return BY_CODE[code];
    const message = (error as { message?: unknown }).message;
    if (typeof message === 'string' && BY_NAME[message]) return BY_NAME[message];
  }
  return 'ble.error.generic';
}

/** Adapter state of the BLE manager → blocking error key; null when usable or transient. */
export function adapterErrorKey(state: string): GaugeErrorKey | null {
  switch (state) {
    case 'PoweredOff':
      return 'ble.error.powered_off';
    case 'Unauthorized':
      return 'ble.error.unauthorized';
    case 'Unsupported':
      return 'ble.error.unsupported';
    default:
      return null; // PoweredOn, or Unknown / Resetting until the next event.
  }
}
