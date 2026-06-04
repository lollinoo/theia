// stringField returns a string field or the API client's existing empty-string fallback.
export function stringField(record: Record<string, unknown>, key: string): string {
  return typeof record[key] === 'string' ? record[key] : '';
}

// stringArray filters unknown arrays down to string entries.
export function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => (typeof item === 'string' ? [item] : []));
}

// permissionKeysArray accepts both raw permission keys and permission objects with a key field.
export function permissionKeysArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => {
    if (typeof item === 'string') {
      return [item];
    }
    if (typeof item === 'object' && item !== null) {
      const key = (item as Record<string, unknown>).key;
      return typeof key === 'string' ? [key] : [];
    }
    return [];
  });
}

// recordField narrows plain object payloads while rejecting arrays and primitives.
export function recordField(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}
