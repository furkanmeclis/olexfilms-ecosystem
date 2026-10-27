import { StockScreen } from '@/features/stock/stock-screen';

/** TEC-958: the organization's own stock from the menu (the center's stock, or a dealer whose tab is Measure). */
export default function CenterStock() {
  return <StockScreen base="/bayi/menu/stock" space="admin" />;
}
