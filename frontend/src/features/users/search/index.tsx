import { Users } from "lucide-react";

import { defineSearchSpec } from "@/features/search-engine/define";

/** Frontend catalog entry for users remote search spec (metadata from API). */
export const usersSearchSpec = defineSearchSpec({
  id: "users",
  labelKey: "search.specs_users",
  icon: Users,
});
