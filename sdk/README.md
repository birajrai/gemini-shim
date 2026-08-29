# gemini-shim (NPM Package)

> Ultra-fast, zero-config Google Gemini AI client & chatbot SDK for JavaScript and TypeScript.

Supports **Node.js**, **Bun**, **Deno**, **Cloudflare Workers**, and modern **Browsers**.

---

## Installation

```bash
npm install gemini-shim
# or
bun add gemini-shim
# or
pnpm add gemini-shim
```

---

## Quickstart

### 1. Simple One-Liner Chat
```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim({
  // Use public edge worker or local proxy
  baseURL: "https://gemini-shim.gauravuchil13.workers.dev/v1", // or "http://127.0.0.1:8081/v1"
  defaultModel: "gemini-3.7-flash",
});

const response = await ai.chat("Explain quantum computing in simple terms.");
console.log(response.text);
```

---

### 2. Real-Time Token Streaming
Stream responses token-by-token directly into stdout or a web response stream:

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

const stream = await ai.stream("Write a short story about an AI discovering music.");

for await (const chunk of stream) {
  process.stdout.write(chunk);
}
```

---

### 3. Multi-Turn Conversational Chatbot
Create stateful chat sessions that automatically retain conversational history:

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

// Initialize chat session with custom system prompt
const bot = ai.createChat({
  systemInstruction: "You are an expert fullstack TypeScript developer.",
});

// First message
const reply1 = await bot.sendMessage("How do I handle SSE in React?");
console.log(reply1.text);

// Follow-up message (preserves previous context)
const reply2 = await bot.sendMessage("Can you show a complete code example?");
console.log(reply2.text);

// Or stream follow-up messages:
for await (const chunk of bot.sendMessageStream("How do I test this?")) {
  process.stdout.write(chunk);
}
```

---

### 4. Building a Next.js / Express AI Chatbot API

#### Next.js Route Handler (`app/api/chat/route.ts`):
```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

export async function POST(req: Request) {
  const { messages } = await req.json();

  const stream = await ai.stream({
    model: "gemini-3.7-flash",
    messages,
  });

  const encoder = new TextEncoder();
  const readable = new ReadableStream({
    async start(controller) {
      for await (const chunk of stream) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });

  return new Response(readable, {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}
```

---

## Supported Models

| Model ID | Description | Best For |
|---|---|---|
| `gemini-3.7-flash` *(default)* | Latest Gemini 3.7 Flash | Coding, general chat, fast agents |
| `gemini-3.5-flash-thinking` | Deep Thinking Mode | Complex problem solving, math, logic |
| `gemini-3.6-flash` | Balanced Flash model | General tasks |
| `gemini-3.1-pro` | Gemini Pro model | High-reasoning tasks |
| `gemini-flash-lite` | Ultra-lightweight model | Low latency classification |

---

## Configuration Options

```typescript
const ai = new GeminiShim({
  // OpenAI-compatible endpoint URL
  baseURL: "https://gemini-shim.gauravuchil13.workers.dev/v1", // or "http://127.0.0.1:8081/v1"

  // Default model for all requests
  defaultModel: "gemini-3.7-flash",

  // Optional API key if your server is secured
  apiKey: "sk-your-key",

  // Optional Google session cookie for authenticated routing
  cookie: "__Secure-1PSID=...; SAPISID=...",

  // Request timeout in milliseconds (default: 60000)
  timeoutMs: 60000,
});
```

---

## License

MIT © [birajrai](https://github.com/birajrai/gemini-shim)
