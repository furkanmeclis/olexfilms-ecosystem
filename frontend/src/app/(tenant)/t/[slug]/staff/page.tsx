import { StaffPage } from "@/features/staff-reports";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <StaffPage slug={slug} />;
}
