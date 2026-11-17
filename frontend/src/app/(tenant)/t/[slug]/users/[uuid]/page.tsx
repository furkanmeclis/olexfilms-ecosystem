"use client";

import { useParams, useSearchParams } from "next/navigation";

import { MemberDetailPage } from "@/features/members/components/member-detail-page";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  const search = useSearchParams();
  return (
    <MemberDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
      initialTab={search.get("tab") ?? undefined}
    />
  );
}
