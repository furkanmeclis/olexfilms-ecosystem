import { Redirect } from 'expo-router';

import { OnboardingScreen } from '@/features/onboarding/onboarding-screen';
import { useOnboarding } from '@/features/onboarding/onboarding-state';

/** First run shows the customer onboarding; afterwards (and after sign-out) the phone sign-in. */
export default function PublicIndex() {
  const seen = useOnboarding((s) => s.seen);
  return seen ? <Redirect href={{ pathname: '/sign-in', params: { mode: 'phone' } }} /> : <OnboardingScreen mode="first-run" />;
}
