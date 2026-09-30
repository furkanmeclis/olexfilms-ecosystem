import { OrganizationEditPage } from "@/features/organizations";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformOrganizationEditPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <OrganizationEditPage uuid={uuid} />;
}
