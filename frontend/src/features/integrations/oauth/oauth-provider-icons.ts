import type { LucideIcon } from "lucide-react";

import { appleNavIcon } from "@/components/icons/apple-icon";
import { facebookNavIcon } from "@/components/icons/facebook-icon";
import { googleNavIcon } from "@/components/icons/google-icon";

import type { OAuthProviderSlug } from "./services/oauth-provider.service";

export const oauthProviderIcons: Record<OAuthProviderSlug, LucideIcon> = {
  google: googleNavIcon,
  facebook: facebookNavIcon,
  apple: appleNavIcon,
};
