import { RoleDetailPage } from "@/features/roles/components/role-detail-page";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformRoleDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <RoleDetailPage uuid={uuid} />;
}
