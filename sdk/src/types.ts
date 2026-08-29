export type GeminiModel =
  | "gemini-3.7-flash"
  | "gemini-3.6-flash"
  | "gemini-3.5-flash"
  | "gemini-3.5-flash-thinking"
  | "gemini-3.1-pro"
  | "gemini-3.1-pro-enhanced"
  | "gemini-auto"
  | "gemini-3.5-flash-thinking-lite"
  | "gemini-flash-lite"
  | (string & {});

export type MessageRole = "system" | "user" | "assistant" | "tool";

export interface ChatMessage {
  role: MessageRole;
  content: string;
  name?: string;
  tool_calls?: ToolCall[];
}

export interface ToolCall {
  id: string;
  type: "function";
  function: {
    name: string;
    arguments: string;
  };
}

export interface FunctionDeclaration {
  name: string;
  description?: string;
  parameters?: Record<string, unknown>;
}

export interface Tool {
  type: "function";
  function: FunctionDeclaration;
}

export interface GeminiShimOptions {
  /**
   * Optional custom endpoint URL. If omitted, the SDK communicates
   * directly with Gemini Web natively with ZERO servers or proxies required.
   */
  baseURL?: string;

  /**
   * Alias for baseURL.
   */
  endpoint?: string;

  /**
   * Optional API Key if using a secured proxy instance.
   */
  apiKey?: string;

  /**
   * Default model to use for requests.
   * Default: "gemini-3.7-flash"
   */
  defaultModel?: GeminiModel;

  /**
   * Optional Google Gemini cookie (e.g. "__Secure-1PSID=...; SAPISID=...")
   * for authenticated multi-account or Pro model routing.
   */
  cookie?: string;

  /**
   * Optional multi-account user index (e.g. "0", "1") when using multiple Google accounts.
   */
  authUser?: string;

  /**
   * Request timeout in milliseconds.
   * Default: 60000 (60s)
   */
  timeoutMs?: number;

  /**
   * Custom fetch implementation.
   */
  fetch?: typeof globalThis.fetch;
}

export interface ChatCompletionOptions {
  model?: GeminiModel;
  messages?: ChatMessage[];
  temperature?: number;
  tools?: Tool[];
  tool_choice?: "auto" | "none" | "required" | { type: "function"; function: { name: string } };
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

export interface ChatResponse {
  id: string;
  model: string;
  text: string;
  toolCalls?: ToolCall[];
  usage?: {
    promptTokens: number;
    completionTokens: number;
    totalTokens: number;
  };
  raw: unknown;
}

export interface CreateChatSessionOptions {
  model?: GeminiModel;
  systemInstruction?: string;
  history?: ChatMessage[];
  tools?: Tool[];
}
