import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** Module level; keys and levels come from the backend catalog only. */
export type ModuleLevel = Schemas["ModuleLevel"];
export type ModuleState = Schemas["ModuleState"];
export type FeatureList = Schemas["EnvelopeFeatureList"]["data"];
/** TEC-508: a module of GET /v1/features with description, price, request. */
export type FeatureListItem = Schemas["FeatureListItem"];
export type ModulePrice = Schemas["ModulePrice"];
export type ModuleRequestStatus = Schemas["ModuleRequestStatus"];
export type ModuleRequestSummary = Schemas["ModuleRequestSummary"];
export type ModuleRequestRow = Schemas["ModuleRequestRow"];
export type DealerModuleRow = Schemas["DealerModuleRow"];
export type DealerStandardEntry = Schemas["DealerStandardEntry"];
export type PlatformModule = Schemas["PlatformModule"];
export type PlatformModulePatch = Schemas["PlatformModulePatch"];
