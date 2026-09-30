import type {
  SearchCatalog,
  SearchSpecDef,
} from "@/features/search-engine/types";

export function defineSearchSpec(spec: SearchSpecDef): SearchSpecDef {
  return spec;
}

export function defineSearchCatalog(catalog: SearchCatalog): SearchCatalog {
  return catalog;
}
