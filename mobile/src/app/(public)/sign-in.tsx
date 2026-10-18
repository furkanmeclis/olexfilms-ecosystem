import { useLocalSearchParams } from 'expo-router';

import { SignInScreen } from '@/features/auth/sign-in-screen';

export default function SignInRoute() {
  const { mode } = useLocalSearchParams<{ mode?: string }>();
  return <SignInScreen mode={mode === 'email' ? 'email' : 'phone'} />;
}
