import { CampaignWizardPage } from "@/features/campaigns/components/campaign-wizard-page";

export default async function NewCampaignPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CampaignWizardPage slug={slug} />;
}
