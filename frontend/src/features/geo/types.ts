import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Country = Schemas["Country"];
export type Province = Schemas["Province"];
export type District = Schemas["District"];
export type Territory = Schemas["Territory"];
export type TerritoryMatch = Schemas["TerritoryMatch"];
export type AssignTerritoryRequest = Schemas["AssignTerritoryRequest"];
export type PlateFormat = Schemas["PlateFormat"];
export type PlateFormatRequest = Schemas["PlateFormatRequest"];
export type PlateCheck = Schemas["PlateCheck"];
