"use client";

import { useParams } from "next/navigation";

import { ReviewQuestionsPage } from "@/features/review-questions";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ReviewQuestionsPage slug={String(params.slug ?? "")} />;
}
