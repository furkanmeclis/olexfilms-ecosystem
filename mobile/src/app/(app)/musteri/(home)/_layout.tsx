import { Stack } from 'expo-router';

import { TabStack } from '@/features/shell/tab-stack';

/** The customer home draws its own top bar (TEC-1019); its native header never mounts. */
export default function CustomerHomeStack() {
  return (
    <TabStack>
      <Stack.Screen name="index" options={{ headerShown: false, headerLargeTitle: false }} />
    </TabStack>
  );
}
