# gemini-shim - Cloudflare Workers Deployment

Deploy **`gemini-shim`** as a serverless edge API on [Cloudflare Workers](https://workers.cloudflare.com/) with zero server maintenance, global edge caching, and real-time SSE streaming.

---

## ⚡ Deployment Methods

### Option 1: Quick Deployment via Wrangler CLI (Recommended)

1. Navigate to the `cloudflare` directory:
   ```bash
   cd cloudflare
   ```

2. Install dependencies and log in to Cloudflare:
   ```bash
   npx wrangler login
   ```

3. Deploy the worker:
   ```bash
   npx wrangler deploy
   ```

---

### Option 2: Web Dashboard (Zero Installation)

1. Log in to the [Cloudflare Dashboard](https://dash.cloudflare.com/).
2. Navigate to **Workers & Pages** -> **Create application** -> **Create Worker**.
3. Name your Worker (e.g., `gemini-shim`) and click **Deploy**.
4. Click **Edit code**, paste the entire contents of [`worker.js`](worker.js), and click **Save and deploy**.

---

## ⚙️ Environment Variables (Settings -> Variables)

You can configure your Worker via the Cloudflare Dashboard (**Settings** > **Variables**) or in `wrangler.toml`:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `DEFAULT_MODEL` | Default model when unspecified | `gemini-3.6-flash` |
| `API_KEYS` | JSON array of authorized keys, e.g. `["sk-my-key"]` | `[]` (Anonymous/Public) |
| `COOKIE_STRING` | Optional Google account cookie (supports multiple separated by `\|`) | `""` |
| `SAPISID` | Optional SAPISID hash key (auto-extracted from `COOKIE_STRING` if omitted) | `""` |
| `FINGERPRINT_JITTER_MS` | Max random delay in ms to simulate realistic browser timing (set to `0` to disable) | `1500` |
| `RETRY_ATTEMPTS` | Upstream retry count on network failure | `3` |
| `REQUEST_TIMEOUT_SEC` | Upstream request timeout | `28` |

---

## 📡 Endpoints & Usage

Once deployed, your Worker provides standard endpoints at `https://gemini-shim.<your-subdomain>.workers.dev`:

### 1. OpenAI Chat Completions (`/v1/chat/completions`)

```bash
curl https://gemini-shim.<your-subdomain>.workers.dev/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.7-flash",
    "messages": [
      {"role": "user", "content": "Explain serverless computing in one sentence."}
    ],
    "stream": true
  }'
```

### 2. OpenAI Models (`/v1/models`)

```bash
curl https://gemini-shim.<your-subdomain>.workers.dev/v1/models
```

### 3. Google Gemini Native (`/v1beta/models/...`)

```bash
curl https://gemini-shim.<your-subdomain>.workers.dev/v1beta/models/gemini-3.7-flash:generateContent \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {"role": "user", "parts": [{"text": "Hello from Cloudflare Workers!"}]}
    ]
  }'
```
