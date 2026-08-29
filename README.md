<div align="center">
  <img src="logo.svg" alt="gemini-shim" width="128" />
  <h1>gemini-shim</h1>
  <p>Ultra-fast lightweight API shim converting Google Gemini Web into OpenAI and Gemini Native API endpoints. Written in <strong>Go + Gin</strong> with native SSE streaming and zero authentication required.</p>
  <p>
    <a href="https://birajrai.github.io/gemini-shim/"><img src="https://img.shields.io/badge/Live_Showcase-birajrai.github.io%2Fgemini--shim-38bdf8?style=flat-square&logo=googlechrome&logoColor=white" alt="Live Showcase" /></a>
    <a href="https://github.com/birajrai/gemini-shim/releases/latest"><img src="https://img.shields.io/github/v/release/birajrai/gemini-shim?style=flat-square&color=6366f1" alt="Latest Release" /></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License" /></a>
  </p>
</div>

---

## Features

- **High Performance**: Built in **Go** using the **Gin** web framework for blazing fast speed, minimal RAM usage (~10MB), and instant startup.
- **Anonymous by Default**: Works out of the box without any Google login or API keys.
- **Optional Cookie Support**: Supports cookie synchronization for Pro models, multi-account routing, and temporary chats.
- **Native SSE Streaming**: Real-time Server-Sent Events (SSE) token streaming.
- **Tool / Function Calling**: Emulates function calling for both OpenAI API and Google Gemini API schemas.
- **Multimodal Support**: Automatic image upload support via Google Scotty resumable protocol.
- **Standard API Endpoints**:
  - OpenAI: `/v1/chat/completions`, `/v1/models`, `/v1/responses`
  - Google Gemini: `/v1beta/models/{model}:generateContent`, `/v1beta/models/{model}:streamGenerateContent`

---

## Quick Start

### Run with Go

```bash
# Clone the repository
git clone https://github.com/birajrai/gemini-shim.git
cd gemini-shim

# Run directly
go run main.go --port 8081

# Or build the binary
go build -o gemini-shim .
./gemini-shim --port 8081
```

### Run with Docker

```bash
docker build -t gemini-shim .
docker run -d -p 8081:8081 --name gemini-shim gemini-shim
```

### Deploy to Cloudflare Workers (Serverless)

You can also deploy `gemini-shim` directly to Cloudflare Workers with zero server requirements:

```bash
cd cloudflare
npx wrangler deploy
```
See the [Cloudflare Deployment Guide](cloudflare/README.md) for web dashboard setup and environment variables.

---

## NPM SDK / Chatbot Library

Build AI chatbots in JavaScript & TypeScript with the official zero-config SDK:

```bash
npm install gemini-shim
```

```typescript
import { GeminiShim } from "gemini-shim";

const ai = new GeminiShim({
  baseURL: "http://127.0.0.1:8081/v1", // or your Cloudflare Worker URL
  defaultModel: "gemini-3.7-flash"
});

// 1. One-liner chat
const res = await ai.chat("Hello Gemini!");
console.log(res.text);

// 2. Real-time token streaming
const stream = await ai.stream("Write a short poem");
for await (const chunk of stream) {
  process.stdout.write(chunk);
}

// 3. Multi-turn Chatbot with memory
const bot = ai.createChat({ systemInstruction: "You are a helpful assistant." });
const r1 = await bot.sendMessage("Hi, I am from Nepal.");
const r2 = await bot.sendMessage("What is my country?"); // Remembers context
```
See the [SDK Documentation](sdk/README.md) for Next.js, Express, and advanced examples.


### Cookie Extraction Extension

Load the unpacked extension in [`extension/`](extension/) into Chrome/Edge to export your Google session authentication to `gemini-auth.json` or copy cookies for Cloudflare Workers with a single click. See [Extension Guide](extension/README.md).

---

## Configuration

You can configure `gemini-shim` using command-line flags, environment variables (`GEMINI_SHIM_CONFIG`), or a JSON configuration file (`config.json` or `~/.config/gemini-shim/config.json`).

```json
{
  "port": 8081,
  "host": "0.0.0.0",
  "retry_attempts": 3,
  "retry_delay_sec": 2,
  "request_timeout_sec": 180,
  "default_model": "gemini-3.6-flash",
  "log_requests": true,
  "cookie_file": "cookie.json",
  "proxy": null,
  "api_keys": ["your-custom-api-key"],
  "temporary_chats": false
}
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | Port to listen on | `8081` |
| `--config` | Path to JSON config file | `./config.json` |
| `--cookie-file` | Path to cookie file | `""` |
| `--proxy` | HTTP/HTTPS proxy URL | `""` |
| `--version` | Display version and exit | `false` |

---

## Supported Models

| Model Name | Description |
|------------|-------------|
| `gemini-3.7-flash` | Latest all-around model (Gemini 3.7 Flash) |
| `gemini-3.6-flash` | All-around model (Gemini 3.6 Flash - Default) |
| `gemini-3.5-flash` | Alias for `gemini-3.6-flash` |
| `gemini-3.5-flash-thinking` | Deep thinking mode (~20k output tokens) |
| `gemini-3.1-pro` | Pro model (requires cookie file for real routing) |
| `gemini-3.1-pro-enhanced` | Pro with enhanced output (experimental) |
| `gemini-auto` | Auto model selection |
| `gemini-3.5-flash-thinking-lite` | Dynamic thinking with adaptive depth |
| `gemini-flash-lite` | Lightweight fast model |

> **Tip**: You can control thinking depth dynamically by appending `@think=0` (deep) or `@think=4` (standard) to model names, e.g. `gemini-3.7-flash@think=0`.

---

## API Usage Examples

### 1. OpenAI Chat Completions (`/v1/chat/completions`)

```bash
curl http://localhost:8081/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.7-flash",
    "messages": [
      {"role": "user", "content": "Explain quantum computing in one sentence."}
    ],
    "stream": false
  }'
```

### 2. Streaming SSE

```bash
curl http://localhost:8081/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.7-flash",
    "messages": [
      {"role": "user", "content": "Write a short poem about coding in Go."}
    ],
    "stream": true
  }'
```

### 3. Google Gemini Native API (`/v1beta/models/...`)

```bash
curl http://localhost:8081/v1beta/models/gemini-3.7-flash:generateContent \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {"role": "user", "parts": [{"text": "Hello Gemini!"}]}
    ]
  }'
```

---

## Testing

Run the full test suite with:

```bash
go test -v ./...
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
