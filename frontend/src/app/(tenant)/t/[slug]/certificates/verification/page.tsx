import { CertificatesPage } from "@/features/certificates";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  await params;
  return <CertificatesPage queue />;
}
