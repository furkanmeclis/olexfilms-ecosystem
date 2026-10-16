import * as Application from 'expo-application';
import { Platform } from 'react-native';

/**
 * Origin of the BFF (`https://<host>/api`). The app calls `/v1/mobile/*` under
 * it with a Bearer token; the Next.js BFF passes the request to Go unchanged.
 */
export const API_URL = (process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:3000/api').replace(/\/$/, '');

/** Mobile API contract major (`X-Mobile-Api-Version`). Bump only with the backend. */
export const MOBILE_API_VERSION = '1';

/** App release (`X-App-Version`), checked by the backend's minimum-version gate. */
export const APP_VERSION = Application.nativeApplicationVersion ?? '0.0.0';

export const PLATFORM = Platform.OS === 'android' ? 'android' : 'ios';
