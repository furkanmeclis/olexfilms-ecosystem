import { ExportDetailPage } from "@/features/io/components/export-detail-page";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformExportDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ExportDetailPage uuid={uuid} />;
}
