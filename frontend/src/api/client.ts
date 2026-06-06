/**
 * Provides frontend API helpers for client endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
export { ServerError, ValidationError } from './errors';
export * from './admin';
export * from './areas';
export * from './auth';
export * from './backup';
export * from './canvas';
export * from './credentials';
export * from './device';
export * from './grafana';
export * from './instanceBackup';
export * from './settings';
export * from './snmp';
export * from './vendor';
export { headersWithCsrf } from './transport';
