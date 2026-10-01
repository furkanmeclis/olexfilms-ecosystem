import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** Module level; keys and levels come from the backend catalog only. */
export type ModuleLevel = Schemas["ModuleLevel"];
export type ModuleState = Schemas["ModuleState"];
export type FeatureList = Schemas["EnvelopeFeatureList"]["data"];
export type DealerModuleRow = Schemas["DealerModuleRow"];
export type DealerStandardEntry = Schemas["DealerStandardEntry"];
export type PlatformModule = Schemas["PlatformModule"];
export type PlatformModulePatch = Schemas["PlatformModulePatch"];
