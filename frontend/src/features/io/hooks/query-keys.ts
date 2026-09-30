import type { ExportJobScope } from "@/features/io/types";

export const ioKeys = {
  exports: {
    all: (scope: ExportJobScope = "platform") => [scope, "exports"] as const,
    lists: (scope: ExportJobScope = "platform") =>
      [...ioKeys.exports.all(scope), "list"] as const,
    list: (params: object, scope: ExportJobScope = "platform") =>
      [...ioKeys.exports.lists(scope), params] as const,
    detail: (uuid: string, scope: ExportJobScope = "platform") =>
      [...ioKeys.exports.all(scope), uuid] as const,
  },
  imports: {
    all: (scope: ExportJobScope = "platform") => [scope, "imports"] as const,
    lists: (scope: ExportJobScope = "platform") =>
      [...ioKeys.imports.all(scope), "list"] as const,
    list: (params: object, scope: ExportJobScope = "platform") =>
      [...ioKeys.imports.lists(scope), params] as const,
    detail: (uuid: string, scope: ExportJobScope = "platform") =>
      [...ioKeys.imports.all(scope), uuid] as const,
  },
  settings: {
    all: (scope: ExportJobScope = "platform") => [scope, "settings"] as const,
  },
  activity: {
    all: ["platform", "activity"] as const,
    meta: () => [...ioKeys.activity.all, "meta"] as const,
    lists: () => [...ioKeys.activity.all, "list"] as const,
    list: (params: object) => [...ioKeys.activity.lists(), params] as const,
  },
};
