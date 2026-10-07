import { CampaignDetailPage } from "@/features/campaigns/components/campaign-detail-page";

export default async function CampaignDetailRoute({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { slug, uuid } = await params;
  return <CampaignDetailPage slug={slug} uuid={uuid} />;
}
