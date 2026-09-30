import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

export default function CmsProfilePreferencesPage() {
  redirect(routes.cms.profile.root);
}
