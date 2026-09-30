import {
  useMutation,
  type DefaultError,
  type UseMutationOptions,
} from "@tanstack/react-query";

/**
 * Thin helper around useMutation with shared defaults.
 * Optimistic update patterns can wrap onMutate/onError/onSettled here later.
 */
export function useAppMutation<
  TData = unknown,
  TError = DefaultError,
  TVariables = void,
  TContext = unknown,
>(options: UseMutationOptions<TData, TError, TVariables, TContext>) {
  return useMutation(options);
}
