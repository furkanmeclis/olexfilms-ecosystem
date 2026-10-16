import { Suspense } from "react";

import { PlatformAIPage } from "@/features/ai-admin";

export default function PlatformAIRoute() {
  return (
    <Suspense>
      <PlatformAIPage />
    </Suspense>
  );
}
