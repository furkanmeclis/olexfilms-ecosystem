import type { DefaultSession } from "next-auth";

declare module "next-auth" {
  interface Session {
    error?: "PasskeySessionError" | "GitHubSessionError" | "OAuthSessionError";
    organizationUuid?: string | null;
    user: DefaultSession["user"] & {
      id: string;
    };
  }

  interface User {
    accessToken?: string;
    refreshToken?: string;
    expiresIn?: number;
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    accessToken?: string;
    refreshToken?: string;
    expiresIn?: number;
    refreshExpiresAt?: string;
    organizationUuid?: string;
    impersonatorUuid?: string;
    error?: "PasskeySessionError" | "GitHubSessionError" | "OAuthSessionError";
  }
}

export {};
