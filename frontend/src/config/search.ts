import { LayoutDashboard } from "lucide-react";

import {
  defineSearchCatalog,
  defineSearchSpec,
} from "@/features/search-engine/define";

/** Local command-palette specs (remote record specs come from GET /v1/search/specs). */
export const searchCatalog = defineSearchCatalog({
  id: "app",
  specs: [
    defineSearchSpec({
      id: "pages",
      labelKey: "search.specs_pages",
      icon: LayoutDashboard,
      local: true,
    }),
  ],
});
