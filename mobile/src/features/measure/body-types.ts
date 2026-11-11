import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

/**
 * Body types of guided measuring, served by the backend
 * (GET /v1/measurement-body-types): code (the measurement's body_type), a
 * key for translations, the NexPTG name, the side view as a PNG data URI and
 * the number of guided points. Immutable, so it is fetched once.
 */
export type BodyType = components['schemas']['MeasurementBodyType'];

export function useBodyTypes() {
  return useQuery({
    queryKey: ['measurements', 'body-types'],
    queryFn: async () => (await unwrap(api.GET('/v1/measurement-body-types'))).items,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: 1,
  });
}

export function bodyTypeOf(list: BodyType[] | undefined, code: string | null | undefined): BodyType | null {
  return list?.find((b) => b.code === code) ?? null;
}
