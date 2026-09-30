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
