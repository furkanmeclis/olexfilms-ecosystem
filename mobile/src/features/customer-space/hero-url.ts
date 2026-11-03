import type { components } from '@/core/api/schema';
import { API_URL } from '@/core/config';

type PortalVehicle = components['schemas']['PortalVehicle'];

/** Model hero → brand hero → default (backend fallback chain, public and cacheable). */
export function heroUrl(v: Pick<PortalVehicle, 'car_brand' | 'car_model'>) {
  if (v.car_model?.uuid) return `${API_URL}/v1/public/vehicle-heroes/models/${v.car_model.uuid}`;
  if (v.car_brand?.uuid) return `${API_URL}/v1/public/vehicle-heroes/brands/${v.car_brand.uuid}`;
  return `${API_URL}/v1/public/vehicle-heroes/default`;
}

export function brandLogoUrl(brandUuid: string) {
  return `${API_URL}/v1/public/brand-logos/${brandUuid}`;
}
