"use client";

import { useQuery } from "@tanstack/react-query";
import { createElement, useCallback, useMemo, useState } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { buildNavPageItems } from "@/features/search-engine/lib/nav-pages";
import { parsePaletteQuery } from "@/features/search-engine/lib/parse-query";
import { readRecentItems } from "@/features/search-engine/lib/recent";
import {
  buildSpecPrefixMap,
  type SpecPrefixEntry,
} from "@/features/search-engine/lib/spec-prefixes";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import {
  fetchSearchHits,
  fetchSearchSpecs,
} from "@/features/search-engine/services/search.service";
import type { PaletteItem } from "@/features/search-engine/types";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

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
  const access = useMemo(
    () => ({
      can,
      canAny,
      org: activeOrg ? { type: activeOrg.type, role: activeOrg.role } : null,
    }),
    [activeOrg, can, canAny],
  );

  const specsQuery = useQuery({
    queryKey: ["search", "specs", variant, tenantSlug],
    queryFn: fetchSearchSpecs,
    staleTime: 60_000,
  });

  const remoteSpecs = useMemo(() => {
    const items = specsQuery.data?.items ?? [];
    return items.filter((spec) => {
      if (spec.tenant_scoped && variant !== "tenant") return false;
      if (!spec.tenant_scoped && variant === "tenant") return false;
      return !spec.permission || can(spec.permission);
    });
  }, [can, specsQuery.data?.items, variant]);

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
    enabled: Boolean(searchText) && specsQuery.data?.enabled !== false,
    staleTime: 15_000,
  });

  const remoteItems = useMemo(() => {
    const hits = remoteQuery.data ?? [];
    return hits.map<PaletteItem>((hit) => ({
      id: `${hit.spec}:${hit.id}`,
      spec: hit.spec,
      label: hit.title,
      description: hit.subtitle,
      href: hit.href,
      iconKey: hit.icon,
      group: t(`search.specs_${hit.spec}`),
    }));
  }, [remoteQuery.data, t]);

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
    push(filteredPages);
    push(remoteItems);
    return groups;
  }, [
    effectiveSpec,
    filteredPages,
    recentQuery.data,
    remoteItems,
    searchText,
    t,
  ]);

  return {
    query,
    setQuery,
    activeSpec,
    setActiveSpec,
    effectiveSpec,
    searchText,
    specOptions,
    groupedItems,
    loading: remoteQuery.isFetching,
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
