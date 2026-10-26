"use client";

import { createContext, useContext } from "react";

import type { DigitStyle } from "@/lib/i18n/digits";

/**
 * The user's digit choice (TEC-1009) for shared UI primitives such as the
 * calendar. LocaleProvider provides it; outside it the default is
 * Arabic-Indic (only Arabic uses it).
 */
export const DigitsContext = createContext<DigitStyle>("arab");

export function useDigits(): DigitStyle {
  return useContext(DigitsContext);
}
