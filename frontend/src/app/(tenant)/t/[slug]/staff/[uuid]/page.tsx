import { StaffDetailPage } from "@/features/staff-reports";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { slug, uuid } = await params;
  return <StaffDetailPage slug={slug} uuid={uuid} />;
}
