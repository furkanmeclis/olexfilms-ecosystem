import { ServiceCatalogPage } from "@/features/service-catalog";

export default async function TenantServiceCatalogRoute({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <ServiceCatalogPage mode="tenant" slug={slug} />;
}
