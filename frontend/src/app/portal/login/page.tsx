import { redirect } from "next/navigation";
import { Suspense } from "react";

import { portalSession } from "@/auth-portal";
import { routes } from "@/config/routes";
import { PortalLogin } from "@/features/portal/components/portal-login";

export default async function PortalLoginPage() {
  const session = await portalSession();
  if (session?.user) redirect(routes.portal.home);
  return (
    <Suspense>
      <PortalLogin />
    </Suspense>
  );
}
