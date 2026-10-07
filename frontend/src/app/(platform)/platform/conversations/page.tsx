import { Suspense } from "react";

import { ConversationsPage } from "@/features/conversations";

export default function PlatformConversationsPage() {
  return (
    <Suspense>
      <ConversationsPage />
    </Suspense>
  );
}
