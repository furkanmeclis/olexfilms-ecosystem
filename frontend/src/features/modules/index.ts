export { FeatureGuard, FeatureDisabled } from "./components/feature-guard";
export { FeaturesPage } from "./components/features-page";
export { PlatformModulesPage } from "./components/platform-modules-page";
export {
  FEATURES_STALE_MS,
  modulesKeys,
  useEnabledFeatures,
  useFeature,
  useFeatures,
} from "./hooks/use-features";
export { modulesService } from "./services/modules.service";
export type * from "./types";
