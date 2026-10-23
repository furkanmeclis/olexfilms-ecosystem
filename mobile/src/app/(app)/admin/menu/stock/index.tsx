import { StockScreen } from '@/features/stock/stock-screen';

/** TEC-958: the center's (or distributor's) own stock, read only (web Stock › Center stock). */
export default function AdminStock() {
  return <StockScreen base="/admin/menu/stock" space="admin" />;
}
