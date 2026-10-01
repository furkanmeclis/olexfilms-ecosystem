"use client";

import { signIn } from "next-auth/react";
import { useQuery } from "@tanstack/react-query";
import { Loader2, Unlink } from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

import { AppleIcon } from "@/components/icons/apple-icon";
import { FacebookIcon } from "@/components/icons/facebook-icon";
import { GitHubIcon } from "@/components/icons/github-icon";
import { GoogleIcon } from "@/components/icons/google-icon";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

type ProviderId = "github" | "google" | "facebook" | "apple";

type OAuthIdentityManagerProps = {
  providers: {
    id: ProviderId;
    enabled: boolean;
  }[];
};

const providerMeta: Record<
  ProviderId,
  { icon: ReactNode; titleKey: string; connectKey: string }
> = {
  github: {
    icon: <GitHubIcon className="size-4" />,
    titleKey: "auth.github.provider",
    connectKey: "auth.github.connect",
  },
  google: {
    icon: <GoogleIcon className="size-4" />,
    titleKey: "auth.google.provider",
    connectKey: "auth.google.connect",
  },
  facebook: {
    icon: <FacebookIcon className="size-4" />,
    titleKey: "auth.facebook.provider",
    connectKey: "auth.facebook.connect",
  },
  apple: {
    icon: <AppleIcon className="size-4" />,
    titleKey: "auth.apple.provider",
    connectKey: "auth.apple.connect",
  },
};

export function OAuthIdentityManager({ providers }: OAuthIdentityManagerProps) {
  const { t, format } = useLocale();
  const [pendingProvider, setPendingProvider] = useState<ProviderId | null>(
    null,
  );

  const identitiesQuery = useQuery({
    queryKey: ["account", "identities"],
    queryFn: async () => {
      try {
        const result = await authService.listIdentities();
        return result.items ?? [];
      } catch (error) {
        toast.error(
          isApiError(error)
            ? error.message || t("common.error_generic")
            : t("common.error_generic"),
        );
        throw error;
      }
    },
  });
  const items = identitiesQuery.data ?? [];
  const loading = identitiesQuery.isPending;

  const connect = async (provider: ProviderId) => {
    setPendingProvider(provider);
    try {
      await signIn(provider, {
        callbackUrl: window.location.href,
        redirect: true,
      });
    } catch {
      toast.error(t("auth.oauth.connect_error"));
      setPendingProvider(null);
    }
  };

  const unlink = async (provider: ProviderId) => {
    setPendingProvider(provider);
    try {
      await authService.unlinkIdentity(provider);
      toast.success(t("auth.oauth.unlink_success"));
      await identitiesQuery.refetch();
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("auth.oauth.unlink_error"));
      } else {
        toast.error(t("auth.oauth.unlink_error"));
      }
    } finally {
      setPendingProvider(null);
    }
  };

  const enabledProviders = providers.filter((p) => p.enabled);

  if (enabledProviders.length === 0) {
    return (
      <Alert>
        <AlertDescription>{t("auth.oauth.disabled_hint")}</AlertDescription>
      </Alert>
    );
  }

  if (loading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-24 w-full rounded-xl" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {enabledProviders.map((provider) => {
        const identity = items.find((item) => item.provider === provider.id);
        const meta = providerMeta[provider.id];
        const pending = pendingProvider === provider.id;

        if (!identity) {
          return (
            <div
              key={provider.id}
              className="flex items-center justify-between gap-3 rounded-xl border p-4"
            >
              <div className="flex items-center gap-2">
                {meta.icon}
                <span className="font-medium">{t(meta.titleKey)}</span>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => connect(provider.id)}
                disabled={pendingProvider !== null}
              >
                {pending ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  meta.icon
                )}
                {pending ? t("auth.oauth.connecting") : t(meta.connectKey)}
              </Button>
            </div>
          );
        }

        return (
          <div
            key={provider.id}
            className="border-border bg-card space-y-3 rounded-xl border p-4 shadow-xs"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  {meta.icon}
                  <span className="font-medium">{t(meta.titleKey)}</span>
                  <Badge variant="secondary">{t("auth.oauth.connected")}</Badge>
                </div>
                {identity.github_login ? (
                  <p className="text-sm">
                    <span className="text-muted-foreground">
                      {t("auth.github.login")}:{" "}
                    </span>
                    <span className="font-mono">{identity.github_login}</span>
                  </p>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {t("auth.oauth.linked_at", {
                    date: format.dateTime(identity.linked_at),
                  })}
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => unlink(provider.id)}
                disabled={pendingProvider !== null}
              >
                {pending ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Unlink aria-hidden />
                )}
                {pending ? t("auth.oauth.unlinking") : t("auth.oauth.unlink")}
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

/** @deprecated Prefer OAuthIdentityManager */
export function GitHubIdentityManager({ enabled }: { enabled: boolean }) {
  return <OAuthIdentityManager providers={[{ id: "github", enabled }]} />;
}
