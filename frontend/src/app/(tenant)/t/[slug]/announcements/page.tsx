"use client";

import { useParams } from "next/navigation";

import { AnnouncementsPage } from "@/features/announcements";

export default function TenantAnnouncementsRoute() {
  const { slug } = useParams<{ slug: string }>();
  return <AnnouncementsPage slug={slug} />;
}
