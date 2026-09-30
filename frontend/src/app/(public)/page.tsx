import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

// No public landing in the F0 base; send visitors to sign-in.
export default function PublicHomePage() {
  redirect(routes.guest.login);
}
