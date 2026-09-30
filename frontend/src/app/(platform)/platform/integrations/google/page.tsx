import { OAuthProviderSettingsPage } from "@/features/integrations/oauth/components/oauth-provider-settings-page";
import { permissions } from "@/config/permissions";

export default function GoogleIntegrationPage() {
  return (
    <OAuthProviderSettingsPage
      provider="google"
      writePermission={permissions.integrations.google.write}
    />
  );
}
