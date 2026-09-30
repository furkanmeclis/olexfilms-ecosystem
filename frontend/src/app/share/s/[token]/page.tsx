import { StoragePublicViewer } from "@/features/storage/components/storage-public-viewer";

type ShareSignedPageProps = {
  params: Promise<{ token: string }>;
};

export default async function ShareSignedPage({
  params,
}: ShareSignedPageProps) {
  const { token } = await params;
  const streamUrl = `/api/v1/public/storage/s/${encodeURIComponent(token)}`;
  const downloadUrl = `${streamUrl}?download=1`;

  return (
    <StoragePublicViewer streamUrl={streamUrl} downloadUrl={downloadUrl} />
  );
}
