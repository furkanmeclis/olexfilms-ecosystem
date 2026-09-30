"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { PromptDialog } from "@/components/dialogs/prompt-dialog";

type ConfirmOptions = {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "default" | "destructive";
};

type PromptOptions = {
  title: string;
  description?: string;
  placeholder?: string;
  confirmLabel?: string;
  cancelLabel?: string;
};

type DialogContextValue = {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
  confirmDelete: (options: Omit<ConfirmOptions, "variant">) => Promise<boolean>;
  prompt: (options: PromptOptions) => Promise<string | null>;
};

const DialogContext = createContext<DialogContextValue | null>(null);

export function DialogProvider({ children }: { children: ReactNode }) {
  const [confirmState, setConfirmState] = useState<{
    options: ConfirmOptions;
    resolve: (value: boolean) => void;
  } | null>(null);

  const [deleteState, setDeleteState] = useState<{
    options: Omit<ConfirmOptions, "variant">;
    resolve: (value: boolean) => void;
  } | null>(null);

  const [promptState, setPromptState] = useState<{
    options: PromptOptions;
    resolve: (value: string | null) => void;
  } | null>(null);

  const confirm = useCallback((options: ConfirmOptions) => {
    return new Promise<boolean>((resolve) => {
      setConfirmState({ options, resolve });
    });
  }, []);

  const confirmDelete = useCallback(
    (options: Omit<ConfirmOptions, "variant">) => {
      return new Promise<boolean>((resolve) => {
        setDeleteState({ options, resolve });
      });
    },
    [],
  );

  const prompt = useCallback((options: PromptOptions) => {
    return new Promise<string | null>((resolve) => {
      setPromptState({ options, resolve });
    });
  }, []);

  const value = useMemo(
    () => ({ confirm, confirmDelete, prompt }),
    [confirm, confirmDelete, prompt],
  );

  return (
    <DialogContext.Provider value={value}>
      {children}
      <ConfirmDialog
        open={Boolean(confirmState)}
        title={confirmState?.options.title ?? ""}
        description={confirmState?.options.description}
        confirmLabel={confirmState?.options.confirmLabel}
        cancelLabel={confirmState?.options.cancelLabel}
        variant={confirmState?.options.variant}
        onConfirm={() => {
          confirmState?.resolve(true);
          setConfirmState(null);
        }}
        onCancel={() => {
          confirmState?.resolve(false);
          setConfirmState(null);
        }}
      />
      <DeleteDialog
        open={Boolean(deleteState)}
        title={deleteState?.options.title ?? ""}
        description={deleteState?.options.description}
        confirmLabel={deleteState?.options.confirmLabel}
        cancelLabel={deleteState?.options.cancelLabel}
        onConfirm={() => {
          deleteState?.resolve(true);
          setDeleteState(null);
        }}
        onCancel={() => {
          deleteState?.resolve(false);
          setDeleteState(null);
        }}
      />
      <PromptDialog
        open={Boolean(promptState)}
        title={promptState?.options.title ?? ""}
        description={promptState?.options.description}
        placeholder={promptState?.options.placeholder}
        confirmLabel={promptState?.options.confirmLabel}
        cancelLabel={promptState?.options.cancelLabel}
        onConfirm={(value) => {
          promptState?.resolve(value);
          setPromptState(null);
        }}
        onCancel={() => {
          promptState?.resolve(null);
          setPromptState(null);
        }}
      />
    </DialogContext.Provider>
  );
}

export function useDialogs() {
  const ctx = useContext(DialogContext);
  if (!ctx) throw new Error("useDialogs must be used within DialogProvider");
  return ctx;
}
