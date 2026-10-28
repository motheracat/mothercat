export type AuthSession = {
  userId: string;
  staffId: string;
  displayName: string;
  role: string;
  permissions: string[];
  isActive: boolean;
};

export type AuthError = {
  code: string;
  message: string;
  requestId?: string;
  fieldErrors?: Record<string, string[]>;
};

export type LoginCredentials = {
  staffId: string;
  password: string;
};

export type PasswordResetRequest = {
  staffId: string;
};

export type PasswordResetPayload = {
  token: string;
  password: string;
  confirmPassword: string;
};

export type AccountActivationPayload = {
  token: string;
  password: string;
  confirmPassword: string;
};
