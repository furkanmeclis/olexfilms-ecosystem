import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { SystemSettingsPage } from "@/features/system-settings";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.settings.read}>
      <SystemSettingsPage />
    </PermissionGuard>
  );
}
