export { GeminiShim } from "./client";
export { ChatSession } from "./chat";
export { parseSSEStream } from "./stream";
export * from "./types";

import { GeminiShim } from "./client";

/**
 * Default convenience instance configured for immediate use.
 */
export const gemini = new GeminiShim();

export default GeminiShim;
