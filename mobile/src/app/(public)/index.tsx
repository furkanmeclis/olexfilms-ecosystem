import { Redirect } from 'expo-router';
import { useState } from 'react';

import { useOnboarding } from '@/core/onboarding';
import { OnboardingScreen } from '@/features/onboarding/onboarding-screen';

/** First run shows the customer onboarding; afterwards (and after sign-out) the phone sign-in. */
export default function PublicIndex() {
  // Read once: markSeen() in the onboarding must not swap this screen for the
  // phone Redirect, which would race its own replace (e.g. to the e-mail mode).
  // Sign-out remounts the (public) group, so the value is fresh there.
  const [seen] = useState(() => useOnboarding.getState().seen);
  return seen ? <Redirect href={{ pathname: '/sign-in', params: { mode: 'phone' } }} /> : <OnboardingScreen mode="first-run" />;
}
