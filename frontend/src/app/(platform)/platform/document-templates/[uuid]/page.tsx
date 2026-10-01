import { DocumentTemplateEditor } from "@/features/document-templates";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformDocumentTemplateEditPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <DocumentTemplateEditor uuid={uuid} />;
}
