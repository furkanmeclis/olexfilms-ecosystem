import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { VehicleBrandDetailPage } from "@/features/vehicle-catalog";

export default async function Page({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return (
    <PermissionGuard permission={permissions.vehicleCatalog.write}>
      <VehicleBrandDetailPage uuid={uuid} />
    </PermissionGuard>
  );
}
