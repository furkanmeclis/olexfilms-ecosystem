export { AddressFields, addressIds } from "./components/address-fields";
export { PlateBadge } from "./components/plate-badge";
export { PlateFormatsPage } from "./components/plate-formats-page";
export { TerritoriesPage } from "./components/territories-page";
export {
  countryName,
  geoKeys,
  useCountries,
  useDistricts,
  usePlateFormats,
  useProvinces,
} from "./hooks/use-geo";
export { formatPlate, matchPlate, normalizePlate } from "./lib/plate";
export { geoService } from "./services/geo.service";
export type * from "./types";
