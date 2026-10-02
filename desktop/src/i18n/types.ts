/**
 * Shape of the auto-extracted dictionaries: nested objects of translatable
 * strings. Function values (parameterized messages) are allowed, mirroring the
 * web app's translator contract. Kept loose so extracted files need no tuning.
 */
export type TranslatorFn = (params: Record<string, unknown>) => unknown;

export type Dictionary = {
  [key: string]: string | TranslatorFn | Dictionary;
};
