import { redirect } from "next/navigation";

import { portalSession } from "@/auth-portal";
import { routes } from "@/config/routes";

/** Portal pages need a portal session (the panel session does not count). */
export default async function PortalAppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const session = await portalSession();
  if (!session?.user) {
    redirect(
      `${routes.portal.login}?next=${encodeURIComponent(routes.portal.home)}`,
    );
  }
  return <>{children}</>;
}
