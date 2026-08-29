import { ChatSession } from "./chat";
import {
  cleanText,
  executeDirectGemini,
  extractResponseText,
  extractTextsFromLine,
  messagesToPrompt,
  parseToolCalls,
  resolveModel,
} from "./engine";
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
  public readonly baseURL?: string;
  public readonly apiKey?: string;
  public readonly defaultModel: GeminiModel;
  public readonly cookie?: string;
  public readonly authUser?: string;
  public readonly timeoutMs: number;
  private customFetch: typeof globalThis.fetch;

  constructor(options: GeminiShimOptions = {}) {
    const rawURL = options.baseURL || options.endpoint || (typeof process !== "undefined" ? process.env?.GEMINI_SHIM_BASE_URL : undefined);

    if (rawURL) {
      this.baseURL = rawURL.replace(/\/+$/, "");
    }
    this.apiKey = options.apiKey || (typeof process !== "undefined" ? process.env?.GEMINI_SHIM_API_KEY : undefined);
    this.defaultModel = options.defaultModel || "gemini-3.7-flash";
    this.cookie = options.cookie || (typeof process !== "undefined" ? process.env?.GEMINI_COOKIE : undefined);
    this.authUser = options.authUser;
    this.timeoutMs = options.timeoutMs || 60000;
    this.customFetch = options.fetch || globalThis.fetch.bind(globalThis);
  }

  /**
   * Send a prompt or messages and receive a complete response in JSON format.
   * Works natively without any proxy server or endpoint.
   */
  async chat(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages"> = {}
  ): Promise<ChatResponse> {
    const payload = this.normalizeInput(input, options);
    const model = payload.model || this.defaultModel;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeoutMs);
    const signal = payload.signal || controller.signal;

    try {
      // 1. If custom endpoint is specified, route through it
      if (this.baseURL) {
        return await this.chatViaEndpoint(payload, model, signal);
      }

      // 2. Otherwise execute natively directly with Google Gemini
      const prompt = messagesToPrompt(payload.messages, payload.tools, payload.tool_choice || "auto");
      const resp = await executeDirectGemini(prompt, model, {
        cookie: this.cookie,
        authUser: this.authUser,
        fetch: this.customFetch,
        signal,
      });

      if (!resp.ok) {
        const errText = await resp.text().catch(() => "Unknown error");
        throw new Error(`Gemini Native HTTP ${resp.status}: ${errText}`);
      }

      const raw = await resp.text();
      let text = extractResponseText(raw);
      let toolCalls = undefined;

      if (payload.tools && payload.tools.length > 0 && text && payload.tool_choice !== "none") {
        const parsed = parseToolCalls(text);
        text = parsed.cleanText;
        toolCalls = parsed.toolCalls.length > 0 ? parsed.toolCalls : undefined;
      }

      return {
        id: `chatcmpl-${Math.random().toString(36).slice(2, 11)}`,
        model,
        text: text || "",
        toolCalls,
        usage: {
          promptTokens: Math.floor(prompt.length / 4),
          completionTokens: Math.floor((text || "").length / 4),
          totalTokens: Math.floor((prompt.length + (text || "").length) / 4),
        },
        raw,
      };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Stream tokens in real time using an async iterator.
   * Works natively without any proxy server or endpoint.
   */
  async stream(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages"> = {}
  ): Promise<AsyncGenerator<string, void, unknown>> {
    const payload = this.normalizeInput(input, options);
    const model = payload.model || this.defaultModel;

    // 1. If custom endpoint is specified, stream through it
    if (this.baseURL) {
      return this.streamViaEndpoint(payload, model);
    }

    // 2. Direct Native streaming
    const prompt = messagesToPrompt(payload.messages, payload.tools, payload.tool_choice || "auto");
    const resp = await executeDirectGemini(prompt, model, {
      cookie: this.cookie,
      authUser: this.authUser,
      fetch: this.customFetch,
      signal: payload.signal,
    });

    if (!resp.ok) {
      const errText = await resp.text().catch(() => "Unknown error");
      throw new Error(`Gemini Stream Native HTTP ${resp.status}: ${errText}`);
    }

    return this.parseDirectNativeStream(resp, payload.signal);
  }

  /**
   * Creates a multi-turn chat session with automatic conversation memory.
   */
  createChat(options: CreateChatSessionOptions = {}): ChatSession {
    return new ChatSession(this, options);
  }

  private async *parseDirectNativeStream(
    response: Response,
    signal?: AbortSignal
  ): AsyncGenerator<string, void, unknown> {
    if (!response.body) {
      throw new Error("Response body is empty");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let emittedRaw = "";

    try {
      while (true) {
        if (signal?.aborted) {
          await reader.cancel();
          return;
        }

        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        while (buffer.includes("\n")) {
          const idx = buffer.indexOf("\n");
          const line = buffer.slice(0, idx).trim();
          buffer = buffer.slice(idx + 1);

          for (const t of extractTextsFromLine(line)) {
            if (t === emittedRaw || emittedRaw.startsWith(t)) continue;
            const delta = cleanText(t.startsWith(emittedRaw) ? t.slice(emittedRaw.length) : t, false);
            emittedRaw = t;
            if (delta) {
              yield delta;
            }
          }
        }
      }
    } finally {
      reader.releaseLock();
    }
  }

  private async chatViaEndpoint(
    payload: ChatCompletionOptions,
    model: GeminiModel,
    signal?: AbortSignal
  ): Promise<ChatResponse> {
    const response = await this.customFetch(`${this.baseURL}/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(this.apiKey ? { Authorization: `Bearer ${this.apiKey}` } : {}),
        ...(this.cookie ? { Cookie: this.cookie } : {}),
        ...payload.headers,
      },
      body: JSON.stringify({
        model,
        messages: payload.messages,
        tools: payload.tools,
        tool_choice: payload.tool_choice,
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
      model: data?.model || model,
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
  }

  private async streamViaEndpoint(
    payload: ChatCompletionOptions,
    model: GeminiModel
  ): Promise<AsyncGenerator<string, void, unknown>> {
    const response = await this.customFetch(`${this.baseURL}/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(this.apiKey ? { Authorization: `Bearer ${this.apiKey}` } : {}),
        ...(this.cookie ? { Cookie: this.cookie } : {}),
        ...payload.headers,
      },
      body: JSON.stringify({
        model,
        messages: payload.messages,
        stream: true,
      }),
      signal: payload.signal,
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => "Unknown error");
      throw new Error(`GeminiShim Stream HTTP ${response.status}: ${errorText}`);
    }

    return parseSSEStream(response, payload.signal);
  }

  private normalizeInput(
    input: string | ChatCompletionOptions,
    options: Omit<ChatCompletionOptions, "messages">
  ): ChatCompletionOptions {
    if (typeof input === "string") {
      return {
        model: options.model || this.defaultModel,
        messages: [{ role: "user", content: input }],
        ...options,
      };
    }
    return {
      model: input.model || this.defaultModel,
      messages: input.messages || [],
      tools: input.tools,
      tool_choice: input.tool_choice,
      headers: input.headers,
      signal: input.signal,
    };
  }
}
