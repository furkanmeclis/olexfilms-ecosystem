"use client";

import { useParams } from "next/navigation";

import { TeamMemberCreatePage } from "@/features/accounts";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TeamMemberCreatePage slug={String(params.slug ?? "")} />;
}
