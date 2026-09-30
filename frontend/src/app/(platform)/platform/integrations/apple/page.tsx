import { OAuthProviderSettingsPage } from "@/features/integrations/oauth/components/oauth-provider-settings-page";
import { permissions } from "@/config/permissions";

export default function AppleIntegrationPage() {
  return (
    <OAuthProviderSettingsPage
      provider="apple"
      writePermission={permissions.integrations.apple.write}
    />
  );
}
