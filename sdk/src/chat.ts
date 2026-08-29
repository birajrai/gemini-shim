import type { GeminiShim } from "./client";
import type { ChatMessage, ChatResponse, CreateChatSessionOptions, GeminiModel } from "./types";

/**
 * Stateful Multi-turn Chat Session for conversational bots.
 */
export class ChatSession {
  private client: GeminiShim;
  private model: GeminiModel;
  private history: ChatMessage[] = [];

  constructor(client: GeminiShim, options: CreateChatSessionOptions = {}) {
    this.client = client;
    this.model = options.model || client.defaultModel;

    if (options.systemInstruction) {
      this.history.push({
        role: "system",
        content: options.systemInstruction,
      });
    }

    if (options.history) {
      this.history.push(...options.history);
    }
  }

  /**
   * Sends a user message and returns the assistant's response,
   * automatically updating conversational history.
   */
  async sendMessage(content: string): Promise<ChatResponse> {
    this.history.push({ role: "user", content });

    const response = await this.client.chat({
      model: this.model,
      messages: this.history,
    });

    this.history.push({
      role: "assistant",
      content: response.text,
      tool_calls: response.toolCalls,
    });

    return response;
  }

  /**
   * Streams the assistant's reply token-by-token while recording the complete
   * output into conversational history upon completion.
   */
  async *sendMessageStream(content: string): AsyncGenerator<string, void, unknown> {
    this.history.push({ role: "user", content });

    let fullText = "";
    const stream = await this.client.stream({
      model: this.model,
      messages: this.history,
    });

    for await (const chunk of stream) {
      fullText += chunk;
      yield chunk;
    }

    this.history.push({
      role: "assistant",
      content: fullText,
    });
  }

  /**
   * Returns a copy of the current message history.
   */
  getHistory(): ChatMessage[] {
    return [...this.history];
  }

  /**
   * Clears the chat history (optionally retaining the system prompt).
   */
  clearHistory(keepSystemPrompt = true): void {
    if (keepSystemPrompt && this.history[0]?.role === "system") {
      this.history = [this.history[0]];
    } else {
      this.history = [];
    }
  }
}
