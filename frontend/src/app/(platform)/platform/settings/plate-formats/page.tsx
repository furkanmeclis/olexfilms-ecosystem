import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { PlateFormatsPage } from "@/features/geo";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.settings.read}>
      <PlateFormatsPage />
    </PermissionGuard>
  );
}
