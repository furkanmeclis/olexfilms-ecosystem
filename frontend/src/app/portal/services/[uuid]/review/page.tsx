import { redirect } from "next/navigation";

import { portalSession } from "@/auth-portal";
import { routes } from "@/config/routes";

/**
 * TEC-353: target of the WhatsApp review link
 * (`/portal/services/{uuid}/review?source=whatsapp_link`, backend
 * ReviewFormTarget). Outside the (app) group so a signed-out customer
 * returns here after the login instead of the portal home; the service
 * page then shows the review form in view and sends source=whatsapp_link.
 */
export default async function PortalServiceReviewLinkPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  const target = `${routes.portal.service(uuid)}?source=whatsapp_link`;
  const session = await portalSession();
  if (!session?.user) {
    redirect(`${routes.portal.login}?next=${encodeURIComponent(target)}`);
  }
  redirect(target);
}
