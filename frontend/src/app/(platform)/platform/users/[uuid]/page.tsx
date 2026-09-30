import { UserDetailPage } from "@/features/users";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformUserDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <UserDetailPage uuid={uuid} />;
}
