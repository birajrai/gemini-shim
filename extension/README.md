# Gemini Shim Session Sync (Browser Extension)

A lightweight browser extension for Chrome, Edge, Brave, and Chromium-based browsers that extracts your Google Gemini session authentication and metadata with a single click.

---

## 🚀 Installation

1. Open your browser's extension manager:
   - **Chrome / Brave**: `chrome://extensions`
   - **Microsoft Edge**: `edge://extensions`
2. Enable **Developer mode** (toggle in the top-right corner).
3. Click **Load unpacked**.
4. Select the `extension` folder inside this repository.

---

## 🎯 Usage

1. Open [https://gemini.google.com/app](https://gemini.google.com/app) and sign in.
2. Click the **Gemini Shim** extension icon in your browser toolbar.
3. Click **Inspect Session** to confirm your session status.
4. Choose an action:
   - **Export `gemini-auth.json`**: Downloads a JSON auth file for local `gemini-shim`.
   - **Copy Cloudflare `COOKIE_STRING`**: Copies the formatted cookie string to your clipboard for Cloudflare Workers deployment.

---

## 🔧 Applying Exported Auth to `gemini-shim`

### Option 1: CLI Flag
```bash
./gemini-shim --cookie-file ./gemini-auth.json
```

### Option 2: `config.json`
```json
{
  "cookie_file": "./gemini-auth.json"
}
```

---

## 🔒 Privacy & Security

This extension runs completely client-side in your browser. It does not send any telemetry, analytics, or session credentials to external third-party servers.
