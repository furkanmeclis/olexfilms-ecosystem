"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export default function Page() {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t("errors.forbidden_title")}</CardTitle>
          <CardDescription>{t("errors.forbidden_body")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline">
            <Link href={routes.guest.login}>{t("errors.back_to_login")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
