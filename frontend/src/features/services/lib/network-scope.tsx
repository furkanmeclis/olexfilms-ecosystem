"use client";

import { useState } from "react";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";

/** "own": the center's own records (its service point); "network": all. */
export type NetworkScope = "own" | "network";

const storageKey = (key: string) => `network-scope:${key}`;

function readScope(key: string, fallback: NetworkScope): NetworkScope {
  if (typeof window === "undefined") return fallback;
  try {
    const v = window.localStorage.getItem(storageKey(key));
    return v === "network" || v === "own" ? v : fallback;
  } catch {
    return fallback;
  }
}

/**
 * TEC-1003: the brand center can work as a service point too, so its
 * service and warranty lists split into its own records (organization =
 * center) and the whole network. TEC-1202: AFM Florya is a dealer, its
 * records are part of the network like any dealer's. `organizationUuid` narrows the list (organization_uuid) when the
 * scope is "own"; other organizations see no switch.
 *
 * `defaultScope` applies until the user picks a side; a saved choice wins
 * (TEC-1075: the service list opens on "network" so dealer names show).
 */
export function useNetworkScope(
  slug: string,
  key: string,
  defaultScope: NetworkScope = "own",
) {
  const org = useActiveOrganization(slug);
  const visible = org?.type === "center";
  const [scope, setScope] = useState<NetworkScope>(() =>
    readScope(key, defaultScope),
  );
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
