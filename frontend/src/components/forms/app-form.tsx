"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import type { ReactNode } from "react";
import {
  FormProvider,
  useForm,
  type DefaultValues,
  type FieldValues,
  type SubmitHandler,
  type UseFormReturn,
} from "react-hook-form";
import type { ZodType } from "zod";

import { ApiError } from "@/lib/api";

type AppFormProps<T extends FieldValues> = {
  schema: ZodType<T>;
  defaultValues?: DefaultValues<T>;
  onSubmit: SubmitHandler<T>;
  children: ReactNode | ((form: UseFormReturn<T>) => ReactNode);
  className?: string;
  id?: string;
};

export function AppForm<T extends FieldValues>({
  schema,
  defaultValues,
  onSubmit,
  children,
  className,
  id,
}: AppFormProps<T>) {
  const form = useForm<T>({
    // zod v4 + RHF resolver typing can be stricter than runtime needs
    resolver: zodResolver(schema as never),
    defaultValues,
  });

  const handleSubmit: SubmitHandler<T> = async (values) => {
    try {
      await onSubmit(values);
    } catch (error) {
      if (error instanceof ApiError && error.isValidation) {
        const fields = error.fieldErrors();
        Object.entries(fields).forEach(([field, message]) => {
          form.setError(field as never, { type: "server", message });
        });
      }
      throw error;
    }
  };

  return (
    <FormProvider {...form}>
      <form
        id={id}
        className={className}
        onSubmit={form.handleSubmit(handleSubmit)}
        noValidate
      >
        {typeof children === "function" ? children(form) : children}
      </form>
    </FormProvider>
  );
}
