"use client";

import * as React from "react";

const MOBILE_BREAKPOINT = 768;
const MOBILE_QUERY = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`;

/** Tailwind `xl` — 1280px */
const XL_QUERY = "(min-width: 1280px)";

export function useIsMobile() {
  return React.useSyncExternalStore(
    (callback) => {
      const mql = window.matchMedia(MOBILE_QUERY);
      mql.addEventListener("change", callback);
      return () => mql.removeEventListener("change", callback);
    },
    () => window.matchMedia(MOBILE_QUERY).matches,
    () => false,
  );
}

export function useIsXl() {
  return React.useSyncExternalStore(
    (callback) => {
      const mql = window.matchMedia(XL_QUERY);
      mql.addEventListener("change", callback);
      return () => mql.removeEventListener("change", callback);
    },
    () => window.matchMedia(XL_QUERY).matches,
    () => true,
  );
}
