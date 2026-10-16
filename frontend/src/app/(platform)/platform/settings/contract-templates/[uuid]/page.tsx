import { ContractTemplateEditor } from "@/features/contracts";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformContractTemplateEditPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <ContractTemplateEditor uuid={uuid} />;
}
