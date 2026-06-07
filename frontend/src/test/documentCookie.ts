/**
 * Sets document.cookie in tests that exercise browser CSRF cookie parsing.
 */
export function setDocumentCookie(cookie: string): void {
  // biome-ignore lint/suspicious/noDocumentCookie: Tests intentionally seed document.cookie for CSRF extraction.
  document.cookie = cookie;
}
