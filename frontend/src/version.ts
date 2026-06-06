/**
 * Defines version frontend module behavior.
 * Keeps this module's responsibility visible before implementation details.
 */
declare const __APP_VERSION__: string;

/** Defines app version constants and helper contracts for the frontend module. */
export const APP_VERSION = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : 'dev';
