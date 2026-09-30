import { OrganizationDetailPage } from "@/features/organizations";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformOrganizationDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <OrganizationDetailPage uuid={uuid} />;
}
