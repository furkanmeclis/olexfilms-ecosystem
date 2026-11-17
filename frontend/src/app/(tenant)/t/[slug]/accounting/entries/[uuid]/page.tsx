import { EntryDetailPage } from "@/features/accounting";

/** TEC-1218: one ledger entry (the accounting layout checks the module). */
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { slug, uuid } = await params;
  return <EntryDetailPage slug={slug} uuid={uuid} />;
}
