export type StepUpMethod = "password" | "passkey" | "totp";

export type StepUpPolicy = {
  ttl_hours: number;
  password_enabled: boolean;
  passkey_enabled: boolean;
  totp_enabled: boolean;
  password_login_totp_required: boolean;
};

export type StepUpStatus = {
  valid: boolean;
  expires_at?: string | null;
  methods: StepUpMethod[];
};

export type StepUpGrant = {
  valid: boolean;
  expires_at: string;
  method: StepUpMethod;
};
