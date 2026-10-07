import { CancelRequestsPage } from "@/features/service-subscriptions";

export default async function TenantSubscriptionCancelRequestsRoute({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CancelRequestsPage slug={slug} />;
}
