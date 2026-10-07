import { CampaignsListPage } from "@/features/campaigns/components/campaigns-list-page";

export default async function CampaignsPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CampaignsListPage slug={slug} />;
}
