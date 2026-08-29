import { ChatSession } from "./chat";
import { parseSSEStream } from "./stream";
import type {
  ChatCompletionOptions,
  ChatMessage,
  ChatResponse,
  CreateChatSessionOptions,
  GeminiModel,
  GeminiShimOptions,
} from "./types";

declare const process: any;

export class GeminiShim {
  public readonly baseURL: string;
  public readonly apiKey: string;
  public readonly defaultModel: GeminiModel;
  public readonly cookie?: string;
  public readonly timeoutMs: number;
  private customFetch: typeof globalThis.fetch;

  constructor(options: GeminiShimOptions = {}) {
    const rawURL =
      options.baseURL ||
      options.endpoint ||
      (typeof process !== "undefined" && process.env?.GEMINI_SHIM_BASE_URL) ||
      "https://gemini-shim.gauravuchil13.workers.dev/v1";

    this.baseURL = rawURL.replace(/\/+$/, "");
    this.apiKey = options.apiKey || (typeof process !== "undefined" ? process.env?.GEMINI_SHIM_API_KEY || "sk-gemini" : "sk-gemini");
    this.defaultModel = options.defaultModel || "gemini-3.7-flash";
    this.cookie = options.cookie;
    this.timeoutMs = options.timeoutMs || 60000;
    this.customFetch = options.fetch || globalThis.fetch.bind(globalThis);
  }

  /**
   * Send a simple prompt or full messages array and receive a complete response.
   */
  async chat(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages"> = {}
  ): Promise<ChatResponse> {
    const requestPayload = this.buildPayload(input, options, false);
    const headers = this.buildHeaders(requestPayload.headers);

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeoutMs);
    const signal = requestPayload.signal || controller.signal;

    try {
      const response = await this.customFetch(`${this.baseURL}/chat/completions`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          model: requestPayload.model || this.defaultModel,
          messages: requestPayload.messages,
          tools: requestPayload.tools,
          tool_choice: requestPayload.tool_choice,
          stream: false,
        }),
        signal,
      });

      if (!response.ok) {
        const errorText = await response.text().catch(() => "Unknown error");
        throw new Error(`GeminiShim HTTP ${response.status}: ${errorText}`);
      }

      const data = await response.json();
      const choice = data?.choices?.[0];
      const message = choice?.message;

      return {
        id: data?.id || `chatcmpl-${Date.now()}`,
        model: data?.model || requestPayload.model || this.defaultModel,
        text: message?.content || "",
        toolCalls: message?.tool_calls,
        usage: data?.usage
          ? {
              promptTokens: data.usage.prompt_tokens,
              completionTokens: data.usage.completion_tokens,
              totalTokens: data.usage.total_tokens,
            }
          : undefined,
        raw: data,
      };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Stream tokens in real time using an async iterator.
   */
  async stream(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages"> = {}
  ): Promise<AsyncGenerator<string, void, unknown>> {
    const requestPayload = this.buildPayload(input, options, true);
    const headers = this.buildHeaders(requestPayload.headers);

    const response = await this.customFetch(`${this.baseURL}/chat/completions`, {
      method: "POST",
      headers,
      body: JSON.stringify({
        model: requestPayload.model || this.defaultModel,
        messages: requestPayload.messages,
        stream: true,
      }),
      signal: requestPayload.signal,
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => "Unknown error");
      throw new Error(`GeminiShim Stream HTTP ${response.status}: ${errorText}`);
    }

    return parseSSEStream(response, requestPayload.signal);
  }

  /**
   * Creates a multi-turn chat session that maintains conversation context.
   */
  createChat(options: CreateChatSessionOptions = {}): ChatSession {
    return new ChatSession(this, options);
  }

  /**
   * Lists all supported models.
   */
  async listModels(): Promise<string[]> {
    const response = await this.customFetch(`${this.baseURL}/models`, {
      method: "GET",
      headers: this.buildHeaders(),
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch models: HTTP ${response.status}`);
    }

    const data = await response.json();
    return (data?.data || []).map((m: { id: string }) => m.id);
  }

  private buildPayload(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages">,
    stream: boolean
  ): ChatCompletionOptions & { stream: boolean } {
    if (typeof input === "string") {
      return {
        model: options.model || this.defaultModel,
        messages: [{ role: "user", content: input }],
        ...options,
        stream,
      };
    }

    return {
      model: input.model || this.defaultModel,
      messages: input.messages || [],
      tools: input.tools,
      tool_choice: input.tool_choice,
      headers: input.headers,
      signal: input.signal,
      stream,
    };
  }

  private buildHeaders(customHeaders?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Authorization: `Bearer ${this.apiKey}`,
      ...customHeaders,
    };

    if (this.cookie) {
      headers["Cookie"] = this.cookie;
    }

    return headers;
  }
}
