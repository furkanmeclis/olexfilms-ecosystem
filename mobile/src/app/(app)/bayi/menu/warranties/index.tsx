import { Redirect } from 'expo-router';

/**
 * TEC-866: the warranty list is closed to dealers (the bayi space is dealer
 * only); a direct link lands on the services list. Warranty details
 * (`./[uuid]`) stay open from the service detail.
 */
export default function WarrantiesListClosed() {
  return <Redirect href="/bayi/services" />;
}
