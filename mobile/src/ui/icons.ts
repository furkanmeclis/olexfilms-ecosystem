import ArrowLeftRightIcon from 'lucide-react-native/icons/arrow-left-right';
import ChevronUpIcon from 'lucide-react-native/icons/chevron-up';
import CircleAlertIcon from 'lucide-react-native/icons/circle-alert';
import ImagePlusIcon from 'lucide-react-native/icons/image-plus';
import ListTreeIcon from 'lucide-react-native/icons/list-tree';
import MonitorSmartphoneIcon from 'lucide-react-native/icons/monitor-smartphone';
import PencilIcon from 'lucide-react-native/icons/pencil';
import BanIcon from 'lucide-react-native/icons/ban';
import BanknoteIcon from 'lucide-react-native/icons/banknote';
import BarcodeIcon from 'lucide-react-native/icons/barcode';
import BluetoothIcon from 'lucide-react-native/icons/bluetooth';
import BoxesIcon from 'lucide-react-native/icons/boxes';
import BuildingIcon from 'lucide-react-native/icons/building';
import CalendarDaysIcon from 'lucide-react-native/icons/calendar-days';
import CameraIcon from 'lucide-react-native/icons/camera';
import CarFrontIcon from 'lucide-react-native/icons/car-front';
import ChartColumnIcon from 'lucide-react-native/icons/chart-column';
import CheckIcon from 'lucide-react-native/icons/check';
import ChevronDownIcon from 'lucide-react-native/icons/chevron-down';
import ClipboardCheckIcon from 'lucide-react-native/icons/clipboard-check';
import ClipboardListIcon from 'lucide-react-native/icons/clipboard-list';
import ClockIcon from 'lucide-react-native/icons/clock';
import CloudOffIcon from 'lucide-react-native/icons/cloud-off';
import DownloadIcon from 'lucide-react-native/icons/download';
import ExternalLinkIcon from 'lucide-react-native/icons/external-link';
import FileTextIcon from 'lucide-react-native/icons/file-text';
import FlashlightIcon from 'lucide-react-native/icons/flashlight';
import GaugeIcon from 'lucide-react-native/icons/gauge';
import GlobeIcon from 'lucide-react-native/icons/globe';
import HammerIcon from 'lucide-react-native/icons/hammer';
import InboxIcon from 'lucide-react-native/icons/inbox';
import InfoIcon from 'lucide-react-native/icons/info';
import KeyRoundIcon from 'lucide-react-native/icons/key-round';
import KeyboardIcon from 'lucide-react-native/icons/keyboard';
import LayoutGridIcon from 'lucide-react-native/icons/layout-grid';
import MailIcon from 'lucide-react-native/icons/mail';
import MapPinIcon from 'lucide-react-native/icons/map-pin';
import MegaphoneIcon from 'lucide-react-native/icons/megaphone';
import NetworkIcon from 'lucide-react-native/icons/network';
import PackageIcon from 'lucide-react-native/icons/package';
import PackageCheckIcon from 'lucide-react-native/icons/package-check';
import PackagePlusIcon from 'lucide-react-native/icons/package-plus';
import PhoneIcon from 'lucide-react-native/icons/phone';
import PlusIcon from 'lucide-react-native/icons/plus';
import QrCodeIcon from 'lucide-react-native/icons/qr-code';
import RulerIcon from 'lucide-react-native/icons/ruler';
import ScanLineIcon from 'lucide-react-native/icons/scan-line';
import SearchIcon from 'lucide-react-native/icons/search';
import SettingsIcon from 'lucide-react-native/icons/settings';
import Share2Icon from 'lucide-react-native/icons/share-2';
import ShieldCheckIcon from 'lucide-react-native/icons/shield-check';
import ShoppingCartIcon from 'lucide-react-native/icons/shopping-cart';
import SmartphoneIcon from 'lucide-react-native/icons/smartphone';
import TargetIcon from 'lucide-react-native/icons/target';
import TrashIcon from 'lucide-react-native/icons/trash';
import TriangleAlertIcon from 'lucide-react-native/icons/triangle-alert';
import TruckIcon from 'lucide-react-native/icons/truck';
import UserRoundIcon from 'lucide-react-native/icons/user-round';
import UserRoundPlusIcon from 'lucide-react-native/icons/user-round-plus';
import UsersIcon from 'lucide-react-native/icons/users';
import WarehouseIcon from 'lucide-react-native/icons/warehouse';
import WrenchIcon from 'lucide-react-native/icons/wrench';
import XIcon from 'lucide-react-native/icons/x';
import ChevronLeftIcon from 'lucide-react-native/icons/chevron-left';
import ChevronRightIcon from 'lucide-react-native/icons/chevron-right';
import type { SFSymbol } from 'expo-symbols';
import { I18nManager } from 'react-native';

/**
 * Icons used by the app, imported one file each. Metro does not tree-shake, so
 * importing from the package root would ship all ~1,600 Lucide icons.
 * Add an icon here, then import it from '@/ui/icons'.
 */
import type { LucideIcon } from 'lucide-react-native';

/** Semantic icon concepts. Screens ask for a concept; the registry picks the glyph per platform. */
export type IconName =
  | 'back'
  | 'forward'
  | 'chevronDown'
  | 'close'
  | 'check'
  | 'add'
  | 'delete'
  | 'share'
  | 'externalLink'
  | 'download'
  | 'search'
  | 'scan'
  | 'barcode'
  | 'qr'
  | 'camera'
  | 'flashlight'
  | 'keyboard'
  | 'customer'
  | 'customers'
  | 'vehicle'
  | 'service'
  | 'warranty'
  | 'warrantyClaim'
  | 'appointment'
  | 'calendar'
  | 'order'
  | 'cart'
  | 'stock'
  | 'package'
  | 'packageIn'
  | 'packageCheck'
  | 'warehouse'
  | 'truck'
  | 'location'
  | 'report'
  | 'chart'
  | 'accounting'
  | 'task'
  | 'announcement'
  | 'lead'
  | 'measure'
  | 'gauge'
  | 'tools'
  | 'building'
  | 'network'
  | 'spaces'
  | 'settings'
  | 'phone'
  | 'mail'
  | 'password'
  | 'shield'
  | 'device'
  | 'bluetooth'
  | 'offline'
  | 'info'
  | 'alert'
  | 'blocked'
  | 'clock'
  | 'target'
  | 'inbox'
  | 'globe'
  | 'transfer'
  | 'chevronUp'
  | 'error'
  | 'addPhoto'
  | 'hierarchy'
  | 'devices'
  | 'edit';

/** iOS renders the SF Symbol, Android (and iOS fallback) renders the Lucide glyph. */
export const ICONS: Record<IconName, { sf: SFSymbol; lucide: LucideIcon }> = {
  back: { sf: 'chevron.backward', lucide: ChevronLeftIcon },
  forward: { sf: 'chevron.forward', lucide: ChevronRightIcon },
  chevronDown: { sf: 'chevron.down', lucide: ChevronDownIcon },
  close: { sf: 'xmark', lucide: XIcon },
  check: { sf: 'checkmark', lucide: CheckIcon },
  add: { sf: 'plus', lucide: PlusIcon },
  delete: { sf: 'trash', lucide: TrashIcon },
  share: { sf: 'square.and.arrow.up', lucide: Share2Icon },
  externalLink: { sf: 'arrow.up.right.square', lucide: ExternalLinkIcon },
  download: { sf: 'arrow.down.circle', lucide: DownloadIcon },
  search: { sf: 'magnifyingglass', lucide: SearchIcon },
  scan: { sf: 'viewfinder', lucide: ScanLineIcon },
  barcode: { sf: 'barcode', lucide: BarcodeIcon },
  qr: { sf: 'qrcode', lucide: QrCodeIcon },
  camera: { sf: 'camera', lucide: CameraIcon },
  flashlight: { sf: 'flashlight.on.fill', lucide: FlashlightIcon },
  keyboard: { sf: 'keyboard', lucide: KeyboardIcon },
  customer: { sf: 'person', lucide: UserRoundIcon },
  customers: { sf: 'person.2', lucide: UsersIcon },
  vehicle: { sf: 'car', lucide: CarFrontIcon },
  service: { sf: 'wrench.and.screwdriver', lucide: WrenchIcon },
  warranty: { sf: 'checkmark.shield', lucide: ShieldCheckIcon },
  warrantyClaim: { sf: 'checklist.checked', lucide: ClipboardCheckIcon },
  appointment: { sf: 'calendar.badge.clock', lucide: CalendarDaysIcon },
  calendar: { sf: 'calendar', lucide: CalendarDaysIcon },
  order: { sf: 'list.clipboard', lucide: ClipboardListIcon },
  cart: { sf: 'cart', lucide: ShoppingCartIcon },
  stock: { sf: 'shippingbox', lucide: BoxesIcon },
  package: { sf: 'shippingbox', lucide: PackageIcon },
  packageIn: { sf: 'shippingbox.and.arrow.backward', lucide: PackagePlusIcon },
  packageCheck: { sf: 'shippingbox.circle', lucide: PackageCheckIcon },
  warehouse: { sf: 'building.2', lucide: WarehouseIcon },
  truck: { sf: 'truck.box', lucide: TruckIcon },
  location: { sf: 'mappin.and.ellipse', lucide: MapPinIcon },
  report: { sf: 'doc.text', lucide: FileTextIcon },
  chart: { sf: 'chart.bar.xaxis', lucide: ChartColumnIcon },
  accounting: { sf: 'banknote', lucide: BanknoteIcon },
  task: { sf: 'checklist', lucide: ClipboardListIcon },
  announcement: { sf: 'megaphone', lucide: MegaphoneIcon },
  lead: { sf: 'person.crop.circle.badge.plus', lucide: UserRoundPlusIcon },
  measure: { sf: 'ruler', lucide: RulerIcon },
  gauge: { sf: 'gauge.with.dots.needle.67percent', lucide: GaugeIcon },
  tools: { sf: 'hammer', lucide: HammerIcon },
  building: { sf: 'building.columns', lucide: BuildingIcon },
  network: { sf: 'network', lucide: NetworkIcon },
  spaces: { sf: 'square.grid.2x2', lucide: LayoutGridIcon },
  settings: { sf: 'gearshape', lucide: SettingsIcon },
  phone: { sf: 'phone', lucide: PhoneIcon },
  mail: { sf: 'envelope', lucide: MailIcon },
  password: { sf: 'lock', lucide: KeyRoundIcon },
  shield: { sf: 'lock.shield', lucide: ShieldCheckIcon },
  device: { sf: 'iphone', lucide: SmartphoneIcon },
  bluetooth: { sf: 'dot.radiowaves.left.and.right', lucide: BluetoothIcon },
  offline: { sf: 'icloud.slash', lucide: CloudOffIcon },
  info: { sf: 'info.circle', lucide: InfoIcon },
  alert: { sf: 'exclamationmark.triangle', lucide: TriangleAlertIcon },
  blocked: { sf: 'nosign', lucide: BanIcon },
  clock: { sf: 'clock', lucide: ClockIcon },
  target: { sf: 'scope', lucide: TargetIcon },
  inbox: { sf: 'tray', lucide: InboxIcon },
  globe: { sf: 'globe', lucide: GlobeIcon },
  transfer: { sf: 'arrow.left.arrow.right', lucide: ArrowLeftRightIcon },
  chevronUp: { sf: 'chevron.up', lucide: ChevronUpIcon },
  error: { sf: 'exclamationmark.circle', lucide: CircleAlertIcon },
  addPhoto: { sf: 'photo.badge.plus', lucide: ImagePlusIcon },
  hierarchy: { sf: 'list.bullet.indent', lucide: ListTreeIcon },
  devices: { sf: 'macbook.and.iphone', lucide: MonitorSmartphoneIcon },
  edit: { sf: 'pencil', lucide: PencilIcon },
};

/** Lucide glyph for a concept; `back`/`forward` mirror in RTL (direction is fixed per launch). */
export function lucideFor(name: IconName): LucideIcon {
  if (I18nManager.isRTL && name === 'back') return ChevronRightIcon;
  if (I18nManager.isRTL && name === 'forward') return ChevronLeftIcon;
  return ICONS[name].lucide;
}
