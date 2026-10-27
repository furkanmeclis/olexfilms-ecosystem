"use client";

import { useParams } from "next/navigation";

import { ContractTemplateEditor } from "@/features/contracts";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ContractTemplateEditor
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
