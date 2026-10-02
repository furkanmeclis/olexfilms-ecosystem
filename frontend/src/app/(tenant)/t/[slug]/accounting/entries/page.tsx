import { EntriesPage } from "@/features/accounting";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { slug } = await params;
  const { cari } = await searchParams;
  const initialCari = typeof cari === "string" && UUID.test(cari) ? cari : "";
  return <EntriesPage slug={slug} initialCari={initialCari} />;
}
