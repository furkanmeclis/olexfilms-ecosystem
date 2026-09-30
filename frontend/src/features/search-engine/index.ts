export {
  defineSearchCatalog,
  defineSearchSpec,
} from "@/features/search-engine/define";
export { CommandPalette } from "@/features/search-engine/components/command-palette";
export { SearchTrigger } from "@/features/search-engine/components/search-trigger";
export {
  CommandPaletteProvider,
  useCommandPalette,
} from "@/features/search-engine/providers/command-palette-provider";
export type {
  PaletteItem,
  RemoteSearchSpec,
  SearchCatalog,
  SearchHit,
  SearchSpecDef,
} from "@/features/search-engine/types";
