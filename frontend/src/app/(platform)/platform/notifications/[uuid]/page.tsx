import { NotificationDetailPage } from "@/features/notifications";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformNotificationDetailRoute({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <NotificationDetailPage uuid={uuid} />;
}
