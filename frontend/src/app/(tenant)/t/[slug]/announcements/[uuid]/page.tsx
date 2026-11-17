"use client";

import { useParams } from "next/navigation";

import { AnnouncementDetailPage } from "@/features/announcements";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <AnnouncementDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
