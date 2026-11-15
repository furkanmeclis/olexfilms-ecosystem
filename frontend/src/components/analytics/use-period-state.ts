"use client";

import { useCallback, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import {
  DEFAULT_PERIOD,
  type PeriodState,
  parsePeriodParams,
  periodToQuery,
  writePeriodParams,
} from "./period";

/**
 * PeriodBar state kept in the URL (`?period=30d&granularity=week&compare=previous`).
 * Returns the state, a setter (router.replace, no scroll) and the API query.
 */
export function usePeriodState(defaults: PeriodState = DEFAULT_PERIOD) {
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const search = params.toString();
  // defaults is usually an inline literal: compare it by value.
  const defaultsKey = JSON.stringify(defaults);
  const stableDefaults = useMemo(
    () => JSON.parse(defaultsKey) as PeriodState,
    [defaultsKey],
  );

  const state = useMemo(
    () => parsePeriodParams(new URLSearchParams(search), stableDefaults),
    [search, stableDefaults],
  );

  const setState = useCallback(
    (next: PeriodState) => {
      const qs = writePeriodParams(search, next, stableDefaults).toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [router, pathname, search, stableDefaults],
  );

  const query = useMemo(() => periodToQuery(state), [state]);
  return [state, setState, query] as const;
}
