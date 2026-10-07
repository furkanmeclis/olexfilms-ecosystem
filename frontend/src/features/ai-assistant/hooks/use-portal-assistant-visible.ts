"use client";

import { useQuery } from "@tanstack/react-query";

import { assistantQueryKey } from "./use-assistant-chat";
import {
  PORTAL_ASSISTANT_SCOPE,
  portalTransport,
} from "../lib/portal-transport";

/** Whether the portal menu shows the assistant (module on, customer realm). */
export function usePortalAssistantVisible(): boolean {
  const status = useQuery({
    queryKey: [...assistantQueryKey(PORTAL_ASSISTANT_SCOPE), "status"],
    queryFn: () => portalTransport.status(),
    retry: false,
    staleTime: 60_000,
  });
  return Boolean(status.data?.enabled && status.data.allowed);
}
