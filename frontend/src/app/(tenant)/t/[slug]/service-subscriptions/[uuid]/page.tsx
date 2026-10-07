import { SubscriptionDetailPage } from "@/features/service-subscriptions";

export default async function TenantServiceSubscriptionDetailRoute({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { slug, uuid } = await params;
  return <SubscriptionDetailPage slug={slug} uuid={uuid} />;
}
