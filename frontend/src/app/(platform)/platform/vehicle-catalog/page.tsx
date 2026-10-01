import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { VehicleBrandsPage } from "@/features/vehicle-catalog";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.vehicleCatalog.write}>
      <VehicleBrandsPage />
    </PermissionGuard>
  );
}
