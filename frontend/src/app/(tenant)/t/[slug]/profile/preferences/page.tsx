import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

type PageProps = {
  params: Promise<{ slug: string }>;
};

export default async function TenantProfilePreferencesPage({
  params,
}: PageProps) {
  const { slug } = await params;
  redirect(routes.tenant.profile.root(slug));
}
