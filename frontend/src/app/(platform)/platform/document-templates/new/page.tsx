import { DocumentTemplateEditor } from "@/features/document-templates";
import {
  DOCUMENT_KINDS,
  DOCUMENT_LANGUAGES,
  type DocumentKind,
} from "@/features/document-templates/services/document-templates.service";

type PageProps = {
  searchParams: Promise<{ kind?: string; language?: string; brand?: string }>;
};

export default async function PlatformDocumentTemplateNewPage({
  searchParams,
}: PageProps) {
  const sp = await searchParams;
  const kind = (DOCUMENT_KINDS as readonly string[]).includes(sp.kind ?? "")
    ? (sp.kind as DocumentKind)
    : "service";
  const language = (DOCUMENT_LANGUAGES as readonly string[]).includes(
    sp.language ?? "",
  )
    ? sp.language!
    : "en";
  return (
    <DocumentTemplateEditor
      kind={kind}
      language={language}
      brandSlug={sp.brand}
    />
  );
}
