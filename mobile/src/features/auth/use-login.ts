import { useMutation } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router } from 'expo-router';

import { isApiError } from '@/core/api/errors';
import { login, verifyPhoneCode } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';

import { usePendingLogin } from './pending-login';

const CODE = /^\d{6}$/;

/** Thrown before any request when a sign-in step is missing its input. */
export class IncompleteLogin extends Error {
  constructor() {
    super('incomplete sign-in input');
  }
}

/** Signs in with a password or a phone code; an account with authenticator 2FA continues on the code screen. */
export function useLogin() {
  return useMutation({
    mutationFn: (totpCode?: string) => {
      const { mode, email, password, phone, code } = usePendingLogin.getState();
      // Backstop for every screen: an incomplete form never reaches the backend.
      if (totpCode !== undefined && !CODE.test(totpCode)) throw new IncompleteLogin();
      if (mode === 'phone') {
        if (!phone || !CODE.test(code)) throw new IncompleteLogin();
        return verifyPhoneCode({ phone, code, totpCode });
      }
      if (!email.trim() || !password.trim()) throw new IncompleteLogin();
      return login({ email, password, totpCode });
    },
    onSuccess: async (result) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      usePendingLogin.getState().clear();
      await useSession.getState().signIn(result);
    },
    onError: (e, totpCode) => {
      if (e instanceof IncompleteLogin) return;
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      if (!totpCode && isApiError(e, 'MFA_REQUIRED')) router.push('/totp');
    },
  });
}
