"use client";

import { useParams } from "next/navigation";

import { ServiceWizardPage } from "@/features/services";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ServiceWizardPage slug={String(params.slug ?? "")} />;
}
