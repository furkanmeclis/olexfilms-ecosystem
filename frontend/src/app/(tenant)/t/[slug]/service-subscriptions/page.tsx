import { SubscriptionsPage } from "@/features/service-subscriptions";

export default async function TenantServiceSubscriptionsRoute({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <SubscriptionsPage slug={slug} />;
}
