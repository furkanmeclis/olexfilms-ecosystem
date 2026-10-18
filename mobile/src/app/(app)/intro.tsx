import { OnboardingScreen } from '@/features/onboarding/onboarding-screen';

/** Settings > "Watch the intro": the onboarding again, ending in "Done". */
export default function IntroRoute() {
  return <OnboardingScreen mode="replay" />;
}
