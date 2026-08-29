# gemini-shim (NPM Package)

> Standalone, zero-config Google Gemini AI client & chatbot SDK for JavaScript and TypeScript.

**Works natively with ZERO servers, ZERO proxies, and ZERO endpoints required.**

Supports **Node.js**, **Bun**, **Deno**, and modern **Browsers**.

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

### 1. Zero-Config One-Liner (No Endpoint, No Port Needed)
```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

const response = await ai.chat("Explain quantum computing in 2 sentences.");
console.log(response.text);
```

---

### 2. Real-Time Token Streaming
Stream responses token-by-token natively:

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

const stream = await ai.stream("Write a short story about an AI discovering music.");

for await (const chunk of stream) {
  process.stdout.write(chunk);
}
```

---

### 3. Multi-Turn Conversational Chatbot (With Memory)
Create stateful chat sessions that automatically retain conversational history:

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim();

// Initialize chat session with custom persona/system prompt
const bot = ai.createChat({
  systemInstruction: "You are an expert fullstack TypeScript developer.",
});

// Turn 1
const reply1 = await bot.sendMessage("How do I handle SSE in React?");
console.log(reply1.text);

// Turn 2 (automatically retains memory & context)
const reply2 = await bot.sendMessage("Can you show a complete code example?");
console.log(reply2.text);

// Or stream follow-up messages:
for await (const chunk of bot.sendMessageStream("How do I test this with Jest?")) {
  process.stdout.write(chunk);
}
```

---

### 4. Optional Custom Settings

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim({
  // Default model (gemini-3.7-flash, gemini-3.5-flash-thinking, gemini-3.1-pro)
  defaultModel: "gemini-3.7-flash",

  // Optional: Google session cookie for Pro models / multi-account routing
  cookie: "__Secure-1PSID=...; SAPISID=...",

  // Optional: Custom endpoint URL if you want to route via your own proxy/worker
  // baseURL: "http://127.0.0.1:8081/v1"
});
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

## License

MIT © [birajrai](https://github.com/birajrai/gemini-shim)
