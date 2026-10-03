"use client";

import { Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useId, useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { routes } from "@/config/routes";

export type WarrantyLookupLabels = {
  label: string;
  placeholder: string;
  submit: string;
  errorRequired: string;
};

export type WarrantyLookupFormProps = {
  labels: WarrantyLookupLabels;
  /** Language picked with `?lang=` on the landing page, carried over. */
  lang?: string;
};

/**
 * Landing warranty lookup (TEC-247): sends the visitor to the public
 * warranty page `/garanti/{no}` (TEC-189). An empty entry shows an inline
 * error instead of navigating. Texts come translated from the server.
 */
export function WarrantyLookupForm({ labels, lang }: WarrantyLookupFormProps) {
  const router = useRouter();
  const inputId = useId();
  const errorId = `${inputId}-error`;
  const [value, setValue] = useState("");
  const [error, setError] = useState(false);

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const no = value.trim();
    if (!no) {
      setError(true);
      return;
    }
    setError(false);
    const query = lang ? `?lang=${encodeURIComponent(lang)}` : "";
    router.push(`${routes.public.warranty(no)}${query}`);
  };

  return (
    <form
      noValidate
      onSubmit={onSubmit}
      data-slot="warranty-lookup"
      className="flex w-full flex-col gap-2"
    >
      <label htmlFor={inputId} className="text-sm font-medium">
        {labels.label}
      </label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          id={inputId}
          name="warranty_no"
          value={value}
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          placeholder={labels.placeholder}
          aria-invalid={error || undefined}
          aria-describedby={error ? errorId : undefined}
          onChange={(e) => {
            setValue(e.target.value);
            if (error && e.target.value.trim()) setError(false);
          }}
          className="h-11 flex-1"
        />
        <Button type="submit" size="lg" className="h-11">
          <Search className="size-4" aria-hidden />
          {labels.submit}
        </Button>
      </div>
      {error ? (
        <p id={errorId} role="alert" className="text-destructive text-sm">
          {labels.errorRequired}
        </p>
      ) : null}
    </form>
  );
}
