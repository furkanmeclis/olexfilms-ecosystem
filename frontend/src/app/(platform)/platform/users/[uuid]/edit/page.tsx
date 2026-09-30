import { UserEditPage } from "@/features/users";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformUserEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <UserEditPage uuid={uuid} />;
}
