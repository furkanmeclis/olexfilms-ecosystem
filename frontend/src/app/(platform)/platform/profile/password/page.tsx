import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

export default function PlatformProfilePasswordPage() {
  redirect(routes.platform.profile.root);
}
