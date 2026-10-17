import { useMutation } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router } from 'expo-router';

import { isApiError } from '@/core/api/errors';
import { login, verifyPhoneCode } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';

import { usePendingLogin } from './pending-login';

/** Signs in with a password or a phone code; an account with authenticator 2FA continues on the code screen. */
export function useLogin() {
  return useMutation({
    mutationFn: (totpCode?: string) => {
      const { mode, email, password, phone, code } = usePendingLogin.getState();
      if (mode === 'phone') return verifyPhoneCode({ phone, code, totpCode });
      return login({ email, password, totpCode });
    },
    onSuccess: async (result) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      usePendingLogin.getState().clear();
      await useSession.getState().signIn(result);
    },
    onError: (e, totpCode) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      if (!totpCode && isApiError(e, 'MFA_REQUIRED')) router.push('/totp');
    },
  });
}
