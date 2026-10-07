import { CampaignApprovalsPage } from "@/features/campaigns/components/campaign-approvals-page";

export default async function CampaignApprovalsRoute({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CampaignApprovalsPage slug={slug} />;
}
