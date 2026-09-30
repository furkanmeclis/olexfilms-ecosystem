import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

export default function PlatformProfilePreferencesPage() {
  redirect(routes.platform.profile.root);
}
