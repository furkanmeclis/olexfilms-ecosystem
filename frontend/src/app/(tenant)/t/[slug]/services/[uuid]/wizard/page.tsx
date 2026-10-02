"use client";

import { useParams } from "next/navigation";

import { ServiceWizardPage } from "@/features/services";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ServiceWizardPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
