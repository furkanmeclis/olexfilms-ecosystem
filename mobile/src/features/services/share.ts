import * as WebBrowser from 'expo-web-browser';
import { Share } from 'react-native';

import { publicWarrantyUrl } from './lib';

/** Opens the public warranty page (the one printed on the certificate). */
export async function openWarranty(code: string) {
  await WebBrowser.openBrowserAsync(publicWarrantyUrl(code));
}

export function shareWarranty(code: string, message: string) {
  const url = publicWarrantyUrl(code);
  void Share.share({ message: `${message}\n${url}`, url });
}
