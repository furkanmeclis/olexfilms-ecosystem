"use client";

import { useState } from "react";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";

/** "own": the center's own records (its service point); "network": all. */
export type NetworkScope = "own" | "network";

const storageKey = (key: string) => `network-scope:${key}`;

function readScope(key: string): NetworkScope {
  if (typeof window === "undefined") return "own";
  try {
    const v = window.localStorage.getItem(storageKey(key));
    return v === "network" ? "network" : "own";
  } catch {
    return "own";
  }
}

/**
 * TEC-1003: the brand center works as a service point too (AFM Florya), so
 * its service and warranty lists split into its own records and the whole
 * network. `organizationUuid` narrows the list (organization_uuid) when the
 * scope is "own"; other organizations see no switch.
 */
export function useNetworkScope(slug: string, key: string) {
  const org = useActiveOrganization(slug);
  const visible = org?.type === "center";
  const [scope, setScope] = useState<NetworkScope>(() => readScope(key));
  const change = (next: NetworkScope) => {
    setScope(next);
    try {
      window.localStorage.setItem(storageKey(key), next);
    } catch {
      // private mode: the choice lasts for the page only
    }
  };
  return {
    visible,
    scope,
    setScope: change,
    organizationUuid: visible && scope === "own" ? (org?.uuid ?? null) : null,
  };
}

export function NetworkScopeToggle({
  scope,
  onChange,
}: {
  scope: NetworkScope;
  onChange: (scope: NetworkScope) => void;
}) {
  const { t } = useLocale();
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={scope}
      aria-label={t("services.network_scope.label")}
      data-testid="network-scope"
      onValueChange={(v) => {
        if (v === "own" || v === "network") onChange(v);
      }}
    >
      <ToggleGroupItem value="own">
        {t("services.network_scope.own")}
      </ToggleGroupItem>
      <ToggleGroupItem value="network">
        {t("services.network_scope.network")}
      </ToggleGroupItem>
    </ToggleGroup>
  );
}
