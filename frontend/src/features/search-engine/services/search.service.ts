import { platformRequest } from "@/lib/api/platform-request";

import type {
  RemoteSearchSpec,
  SearchHit,
} from "@/features/search-engine/types";

type SearchSpecsResponse = {
  items: RemoteSearchSpec[];
  enabled: boolean;
};

type SearchHitsResponse = {
  items: SearchHit[];
};

export async function fetchSearchSpecs(): Promise<SearchSpecsResponse> {
  return platformRequest<SearchSpecsResponse>("GET", "/v1/search/specs");
}

export async function fetchSearchHits(
  q: string,
  spec?: string,
  limit = 20,
): Promise<SearchHit[]> {
  const data = await platformRequest<SearchHitsResponse>("GET", "/v1/search", {
    query: { q, spec, limit },
  });
  return data.items ?? [];
}

export type GlobalSearchGroup = {
  spec: string;
  label_key: string;
  icon?: string | null;
  items: SearchHit[];
};

export type GlobalSearchResponse = {
  enabled: boolean;
  /** `search_disabled` when Meilisearch is off (groups are empty). */
  info: string | null;
  groups: GlobalSearchGroup[];
};

/** TEC-213: grouped search over every index of the active organization. */
export async function fetchGlobalSearch(
  q: string,
  spec?: string,
  limit = 5,
): Promise<GlobalSearchResponse> {
  return platformRequest<GlobalSearchResponse>("GET", "/v1/search/global", {
    query: { q, spec, limit },
  });
}
