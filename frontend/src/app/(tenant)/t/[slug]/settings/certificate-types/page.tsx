"use client";

import { useParams } from "next/navigation";

import { CertificateTypesPage } from "@/features/certificates";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CertificateTypesPage slug={String(params.slug ?? "")} />;
}
