import type { paths } from "@/generated/api";

export type ApiMethod = "get" | "put" | "post" | "patch" | "delete";

/** OpenAPI path templates that declare `method`. */
export type ApiPathFor<M extends ApiMethod> = {
  [P in keyof paths]: [NonNullable<paths[P][M]>] extends [never] ? never : P;
}[keyof paths];

type PathParamNames<P extends string> =
  P extends `${string}{${infer Name}}${infer Rest}`
    ? Name | PathParamNames<Rest>
    : never;

export type ApiPathParams<P extends string> = Record<PathParamNames<P>, string>;

/**
 * Concrete path of an OpenAPI path template (TEC-755). The template must be
 * a key of the generated `paths` that declares `method`, and every `{name}`
 * needs a value; so a service calling an endpoint the backend does not
 * serve fails `pnpm typecheck` instead of 404ing at runtime.
 */
export function apiPath<M extends ApiMethod, P extends ApiPathFor<M>>(
  method: M,
  template: P,
  ...[params]: [PathParamNames<P>] extends [never] ? [] : [ApiPathParams<P>]
): string {
  void method;
  const values = (params ?? {}) as Record<string, string>;
  return template.replace(/\{([^}]+)\}/g, (_, name: string) => {
    const value = values[name];
    if (value === undefined) throw new Error(`apiPath: missing {${name}}`);
    return encodeURIComponent(value);
  });
}
