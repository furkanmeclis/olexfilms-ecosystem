"use client";

import { ImportsPage } from "@/features/io";
import { useParams } from "next/navigation";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ImportsPage scope="tenant" slug={String(params.slug ?? "")} />;
}
