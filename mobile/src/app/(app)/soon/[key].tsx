import { useLocalSearchParams } from 'expo-router';

import { ComingSoon } from '@/features/shell/coming-soon';

export default function Soon() {
  const { key } = useLocalSearchParams<{ key: string }>();
  return <ComingSoon titleKey={`menu:${key}`} />;
}
