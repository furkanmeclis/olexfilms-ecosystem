import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { TerritoriesPage } from "@/features/geo";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.territories.read}>
      <TerritoriesPage />
    </PermissionGuard>
  );
}
