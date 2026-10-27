import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

export default function PublicLayout() {
  const { t } = useTranslation();
  return (
    <Stack screenOptions={{ headerBackTitle: t('common:back'), headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Screen name="sign-in" options={{ headerShown: false }} />
      {/* Sign-in steps draw their own back button (AuthStep). */}
      <Stack.Screen name="phone" options={{ headerShown: false }} />
      <Stack.Screen name="password" options={{ headerShown: false }} />
      <Stack.Screen name="totp" options={{ headerShown: false }} />
    </Stack>
  );
}
