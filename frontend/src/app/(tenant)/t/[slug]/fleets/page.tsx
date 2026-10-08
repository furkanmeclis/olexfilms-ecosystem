import { FleetsListPage } from "@/features/fleets";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <FleetsListPage slug={slug} />;
}
