import { StoragePublicViewer } from "@/features/storage/components/storage-public-viewer";

type ShareSlugPageProps = {
  params: Promise<{ slug: string }>;
};

export default async function ShareSlugPage({ params }: ShareSlugPageProps) {
  const { slug } = await params;
  const streamUrl = `/api/v1/public/storage/${encodeURIComponent(slug)}`;
  const downloadUrl = `${streamUrl}?download=1`;

  return (
    <StoragePublicViewer streamUrl={streamUrl} downloadUrl={downloadUrl} />
  );
}
