import type { ReactNode } from "react";

import { SettingsLayout } from "@/components/layout/settings-layout";

/** TEC-771: platform settings area with its sub-navigation. */
export default function PlatformSettingsLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <SettingsLayout variant="platform">{children}</SettingsLayout>;
}
