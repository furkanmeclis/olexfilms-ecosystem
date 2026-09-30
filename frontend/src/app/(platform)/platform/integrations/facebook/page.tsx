import { OAuthProviderSettingsPage } from "@/features/integrations/oauth/components/oauth-provider-settings-page";
import { permissions } from "@/config/permissions";

export default function FacebookIntegrationPage() {
  return (
    <OAuthProviderSettingsPage
      provider="facebook"
      writePermission={permissions.integrations.facebook.write}
    />
  );
}
