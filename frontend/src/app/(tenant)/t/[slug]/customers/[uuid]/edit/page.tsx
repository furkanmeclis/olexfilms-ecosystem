"use client";

import { useParams } from "next/navigation";

import { CustomerFormPage } from "@/features/customers";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <CustomerFormPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
