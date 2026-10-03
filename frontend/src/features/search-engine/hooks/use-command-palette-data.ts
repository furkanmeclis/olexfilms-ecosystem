"use client";

import { useQuery } from "@tanstack/react-query";
import { createElement, useCallback, useMemo, useState } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { Permission } from "@/config/permissions";
import {
  detectQueryKind,
  prioritizeGroups,
} from "@/features/search-engine/lib/detect-query";
import { buildNavPageItems } from "@/features/search-engine/lib/nav-pages";
import { parsePaletteQuery } from "@/features/search-engine/lib/parse-query";
import { resolveSearchHitHref } from "@/features/search-engine/lib/hit-href";
import { readRecentItems } from "@/features/search-engine/lib/recent";
import {
  buildSpecPrefixMap,
  type SpecPrefixEntry,
} from "@/features/search-engine/lib/spec-prefixes";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import {
  fetchGlobalSearch,
  fetchSearchHits,
  fetchSearchSpecs,
} from "@/features/search-engine/services/search.service";
import type { PaletteItem } from "@/features/search-engine/types";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * TEC-213: record indexes of the tenant global search (GET /v1/search/global)
 * with the list permission each needs. The server checks the permission,
 * scope and module feature again and leaves out what the caller cannot read.
 */
export const RECORD_SPECS = [
  { id: "customers", permission: Permission.CustomersRead, icon: "users" },
  { id: "vehicles", permission: Permission.VehiclesRead, icon: "car" },
  { id: "services", permission: Permission.ServicesRead, icon: "wrench" },
  {
    id: "warranties",
    permission: Permission.WarrantiesRead,
    icon: "shield-check",
  },
  { id: "orders", permission: Permission.OrdersRead, icon: "receipt" },
  {
    id: "organizations",
    permission: Permission.OrganizationsRead,
    icon: "building",
  },
  { id: "stock_units", permission: Permission.StockRead, icon: "barcode" },
] as const;

export function useCommandPaletteData(
  variant: AppLayoutVariant,
  tenantSlug?: string,
) {
  const { t } = useLocale();
  const { can, canAny } = usePermission();
  const [query, setQueryState] = useState("");
  const [activeSpec, setActiveSpec] = useState<string | undefined>();

  const setQuery = useCallback((next: string) => {
    setQueryState(next);
    if (!next.trim()) setActiveSpec(undefined);
  }, []);

  const recentQuery = useQuery({
    queryKey: ["search", "recent-items"],
    queryFn: () => Promise.resolve(readRecentItems()),
    staleTime: Infinity,
  });
  const activeOrg = useActiveOrganization(
    variant === "tenant" ? tenantSlug : null,
  );
  const features = useEnabledFeatures(variant === "tenant" ? tenantSlug : null);
  const access = useMemo(
    () => ({
      can,
      canAny,
      org: activeOrg
        ? { type: activeOrg.type, role: activeOrg.role, features }
        : null,
    }),
    [activeOrg, can, canAny, features],
  );

  const specsQuery = useQuery({
    queryKey: ["search", "specs", variant, tenantSlug],
    queryFn: fetchSearchSpecs,
    staleTime: 60_000,
  });

  const globalMode = variant === "tenant";

  const remoteSpecs = useMemo(() => {
    const items = specsQuery.data?.items ?? [];
    const records = globalMode
      ? RECORD_SPECS.filter((spec) => can(spec.permission)).map((spec) => ({
          id: spec.id,
          label_key: `search.specs_${spec.id}`,
          permission: spec.permission as string,
          icon: spec.icon as string,
        }))
      : [];
    const indexed = items.filter((spec) => {
      // Brand-scoped specs (catalog products, TEC-145) filter on the active
      // organization's brand, so they belong to the tenant shell too.
      const tenantOnly = Boolean(spec.tenant_scoped || spec.brand_scoped);
      if (tenantOnly && variant !== "tenant") return false;
      if (!tenantOnly && variant === "tenant") return false;
      return !spec.permission || can(spec.permission);
    });
    return [...records, ...indexed];
  }, [can, globalMode, specsQuery.data?.items, variant]);

  const prefixMap = useMemo(() => {
    const entries: SpecPrefixEntry[] = [
      {
        id: "pages",
        aliases: ["pages", t("search.prefix_pages")],
      },
    ];
    for (const spec of remoteSpecs) {
      entries.push({
        id: spec.id,
        aliases: [spec.id, t(`search.prefix_${spec.id}`)],
      });
    }
    return buildSpecPrefixMap(entries);
  }, [remoteSpecs, t]);

  const parsed = useMemo(
    () => parsePaletteQuery(query, prefixMap),
    [prefixMap, query],
  );

  const effectiveSpec = activeSpec ?? parsed.spec;
  const searchText = parsed.text;

  const pageItems = useMemo(
    () => buildNavPageItems(variant, access, t, tenantSlug),
    [access, t, tenantSlug, variant],
  );

  const filteredPages = useMemo(() => {
    if (effectiveSpec && effectiveSpec !== "pages") return [];
    if (!searchText) return pageItems.slice(0, 12);
    const q = searchText.toLowerCase();
    return pageItems.filter(
      (item) =>
        item.label.toLowerCase().includes(q) ||
        item.description?.toLowerCase().includes(q) ||
        item.href.toLowerCase().includes(q),
    );
  }, [effectiveSpec, pageItems, searchText]);

  const remoteQuery = useQuery({
    queryKey: ["search", "hits", searchText, effectiveSpec],
    queryFn: () => fetchSearchHits(searchText, effectiveSpec),
    enabled:
      !globalMode && Boolean(searchText) && specsQuery.data?.enabled !== false,
    staleTime: 15_000,
  });

  // TEC-213: the tenant shell searches every record index at once.
  const globalQuery = useQuery({
    queryKey: ["search", "global", tenantSlug, searchText, effectiveSpec],
    queryFn: () => fetchGlobalSearch(searchText, effectiveSpec),
    enabled: globalMode && Boolean(searchText) && effectiveSpec !== "pages",
    staleTime: 15_000,
  });

  const queryKind = useMemo(() => detectQueryKind(searchText), [searchText]);

  const globalItems = useMemo(() => {
    const groups = prioritizeGroups(globalQuery.data?.groups ?? [], queryKind);
    return groups.flatMap((group) =>
      group.items.map<PaletteItem>((hit) => ({
        id: `${hit.spec}:${hit.id}`,
        spec: hit.spec,
        label: hit.title,
        description: hit.subtitle,
        href: resolveSearchHitHref(hit.href, tenantSlug),
        iconKey: hit.icon ?? group.icon ?? undefined,
        group: t(group.label_key),
      })),
    );
  }, [globalQuery.data?.groups, queryKind, t, tenantSlug]);

  const remoteItems = useMemo(() => {
    if (globalMode) return globalItems;
    const hits = remoteQuery.data ?? [];
    return hits.map<PaletteItem>((hit) => ({
      id: `${hit.spec}:${hit.id}`,
      spec: hit.spec,
      label: hit.title,
      description: hit.subtitle,
      // Outside the tenant shell hit links are absolute.
      href: resolveSearchHitHref(hit.href, null),
      iconKey: hit.icon,
      group: t(`search.specs_${hit.spec}`),
    }));
  }, [globalItems, globalMode, remoteQuery.data, t]);

  const specOptions = useMemo(() => {
    const options = [
      {
        id: "pages",
        label: t("search.specs_pages"),
        icon: resolveSearchIcon("pages"),
      },
    ];
    for (const spec of remoteSpecs) {
      options.push({
        id: spec.id,
        label: t(spec.label_key),
        icon: resolveSearchIcon(spec.icon ?? spec.id),
      });
    }
    return options;
  }, [remoteSpecs, t]);

  const groupedItems = useMemo(() => {
    const groups = new Map<string, PaletteItem[]>();
    const push = (items: PaletteItem[]) => {
      for (const item of items) {
        const key = item.group;
        groups.set(key, [...(groups.get(key) ?? []), item]);
      }
    };

    const recent = recentQuery.data ?? [];
    if (!searchText && !effectiveSpec && recent.length > 0) {
      push(
        recent.map((item) => ({ ...item, group: t("search.group_recent") })),
      );
    }
    // A recognised barcode / plate / VIN puts its record group first.
    if (queryKind) {
      push(remoteItems);
      push(filteredPages);
    } else {
      push(filteredPages);
      push(remoteItems);
    }
    return groups;
  }, [
    effectiveSpec,
    filteredPages,
    queryKind,
    recentQuery.data,
    remoteItems,
    searchText,
    t,
  ]);

  const infoText =
    globalMode && globalQuery.data?.info === "search_disabled"
      ? t("search.global_disabled")
      : undefined;

  return {
    query,
    setQuery,
    activeSpec,
    setActiveSpec,
    effectiveSpec,
    searchText,
    specOptions,
    groupedItems,
    loading: globalMode ? globalQuery.isFetching : remoteQuery.isFetching,
    infoText,
    remoteEnabled: specsQuery.data?.enabled ?? true,
    placeholder: t("search.placeholder"),
    emptyText: t("search.empty"),
    footerHint: t("search.footer_hint", {
      prefix: t("search.prefix_users"),
      example: t("search.footer_example"),
    }),
    recentEnabled: !searchText && !effectiveSpec,
    refreshRecent: () => {
      void recentQuery.refetch();
    },
  };
}

export function paletteItemIcon(item: PaletteItem) {
  if (item.icon) return item.icon;
  return createElement(resolveSearchIcon(item.iconKey ?? item.spec), {
    className: "size-4 shrink-0 opacity-80",
  });
}
