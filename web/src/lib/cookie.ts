export function readCookie(name: string): string | null {
  const prefix = `${name}=`;
  const parts = document.cookie ? document.cookie.split('; ') : [];
  for (const part of parts) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return null;
}
