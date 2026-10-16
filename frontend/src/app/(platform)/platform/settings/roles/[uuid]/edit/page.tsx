import { RoleEditPage } from "@/features/roles/components/role-edit-page";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformRoleEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <RoleEditPage uuid={uuid} />;
}
