export function developmentOrigin(value: string | undefined, isPackaged: boolean): string | null {
  if (isPackaged || !value || !/^http:\/\/127\.0\.0\.1:[1-9]\d{0,4}\/?$/.test(value)) return null;
  try {
    return new URL(value).origin;
  } catch {
    return null;
  }
}
