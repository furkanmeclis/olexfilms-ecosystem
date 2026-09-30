import { ImportDetailPage } from "@/features/io/components/import-detail-page";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformImportDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ImportDetailPage uuid={uuid} />;
}
