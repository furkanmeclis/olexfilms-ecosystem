import { Redirect } from 'expo-router';

// Temporary: Görev 9 puts the onboarding gate here.
export default function PublicIndex() {
  return <Redirect href={{ pathname: '/sign-in', params: { mode: 'phone' } }} />;
}
