/**
 * gemini-shim - Cloudflare Workers Edition
 * OpenAI & Gemini API compatible edge proxy for Google Gemini Web.
 * Repository: https://github.com/birajrai/gemini-shim
 * Author: birajrai
 * License: MIT
 */

const DEFAULT_CONFIG = {
  retryAttempts: 3,
  retryDelaySec: 2,
  requestTimeoutSec: 28,
  geminiBl: "boq_assistant-bard-web-server_20260716.08_p0",
  authUser: null,
  xsrfToken: null,
  defaultModel: "gemini-3.6-flash",
  apiKeys: [],
  cookieString: null,
  sapisid: null,
  logRequests: true,
  rateLimit: {
    enabled: true,
    maxRequests: 3000,
    windowSec: 60,
  },
  fingerprintJitterMs: 1500,
};

const MODELS = {
  "gemini-3.7-flash": { mode: 1, think: 4, desc: "Latest all-around model (Gemini 3.7 Flash)" },
  "gemini-3.6-flash": { mode: 1, think: 4, desc: "All-around model (Gemini 3.6 Flash)" },
  "gemini-3.5-flash": { mode: 1, think: 4, desc: "Alias for gemini-3.6-flash" },
  "gemini-3.5-flash-thinking": { mode: 2, think: 0, desc: "Deep thinking mode, longest output" },
  "gemini-3.1-pro": { mode: 3, think: 4, desc: "Pro model (requires cookie for real routing)" },
  "gemini-3.1-pro-enhanced": { mode: 3, think: 4, extra: { 31: 2, 80: 3 }, desc: "Pro with enhanced output" },
  "gemini-auto": { mode: 4, think: 4, desc: "Auto model selection" },
  "gemini-3.5-flash-thinking-lite": { mode: 5, think: 0, desc: "Dynamic thinking with adaptive depth" },
  "gemini-flash-lite": { mode: 6, think: 4, desc: "Lightweight fast model" },
};

const USER_AGENTS = [
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
];
const UA_WEIGHTS = [35, 30, 10, 10, 8, 7];

const ACCEPT_LANGUAGES = [
  "en-US,en;q=0.9",
  "en-GB,en;q=0.9,en-US;q=0.8",
  "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7",
  "en;q=0.8",
];

const rateLimitStore = new Map();

function getRandomItem(items, weights) {
  if (!weights) return items[Math.floor(Math.random() * items.length)];
  let total = weights.reduce((a, b) => a + b, 0);
  let random = Math.random() * total;
  for (let i = 0; i < items.length; i++) {
    if (random < weights[i]) return items[i];
    random -= weights[i];
  }
  return items[0];
}

function getRequestConfig(env) {
  const cfg = { ...DEFAULT_CONFIG };
  if (env.RETRY_ATTEMPTS) cfg.retryAttempts = parseInt(env.RETRY_ATTEMPTS, 10);
  if (env.RETRY_DELAY_SEC) cfg.retryDelaySec = parseInt(env.RETRY_DELAY_SEC, 10);
  if (env.REQUEST_TIMEOUT_SEC) cfg.requestTimeoutSec = parseInt(env.REQUEST_TIMEOUT_SEC, 10);
  if (env.GEMINI_BL) cfg.geminiBl = env.GEMINI_BL;
  if (env.AUTH_USER) cfg.authUser = env.AUTH_USER;
  if (env.XSRF_TOKEN) cfg.xsrfToken = env.XSRF_TOKEN;
  if (env.DEFAULT_MODEL) cfg.defaultModel = env.DEFAULT_MODEL;
  if (env.FINGERPRINT_JITTER_MS) cfg.fingerprintJitterMs = parseInt(env.FINGERPRINT_JITTER_MS, 10);

  if (env.API_KEYS) {
    try {
      cfg.apiKeys = JSON.parse(env.API_KEYS);
    } catch {
      cfg.apiKeys = env.API_KEYS.split(",").map((k) => k.trim()).filter(Boolean);
    }
  }

  cfg.cookieString = env.COOKIE_STRING || null;
  cfg.sapisid = env.SAPISID || null;
  return cfg;
}

function checkRateLimit(ip, cfg) {
  if (!cfg.rateLimit?.enabled) return true;
  const now = Date.now();
  const windowMs = cfg.rateLimit.windowSec * 1000;
  const record = rateLimitStore.get(ip) || { count: 0, resetTime: now + windowMs };

  if (now > record.resetTime) {
    record.count = 1;
    record.resetTime = now + windowMs;
  } else {
    record.count += 1;
  }
  rateLimitStore.set(ip, record);

  if (Math.random() < 0.05) {
    for (const [key, val] of rateLimitStore.entries()) {
      if (now > val.resetTime) rateLimitStore.delete(key);
    }
  }

  return record.count <= cfg.rateLimit.maxRequests;
}

async function sha1Hex(str) {
  const msgUint8 = new TextEncoder().encode(str);
  const hashBuffer = await crypto.subtle.digest("SHA-1", msgUint8);
  return Array.from(new Uint8Array(hashBuffer)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function getAuthHeaders(cfg) {
  const ua = getRandomItem(USER_AGENTS, UA_WEIGHTS);
  const lang = getRandomItem(ACCEPT_LANGUAGES);
  const accountPrefix = cfg.authUser ? `/u/${cfg.authUser}` : "";

  const headers = {
    "Content-Type": "application/x-www-form-urlencoded",
    "Origin": "https://gemini.google.com",
    "Referer": `https://gemini.google.com${accountPrefix}/app`,
    "X-Same-Domain": "1",
    "User-Agent": ua,
    "Accept-Language": lang,
  };

  if (cfg.authUser) {
    headers["X-Goog-AuthUser"] = String(cfg.authUser);
  }

  let cookieStr = cfg.cookieString;
  let sapisid = cfg.sapisid;

  if (cookieStr && cookieStr.includes("|")) {
    const cookies = cookieStr.split("|").map((c) => c.trim()).filter(Boolean);
    cookieStr = getRandomItem(cookies);
  }

  if (!sapisid && cookieStr) {
    const m = cookieStr.match(/SAPISID=([^;]+)/);
    if (m) sapisid = m[1].trim();
  }

  if (cookieStr) headers["Cookie"] = cookieStr;
  if (sapisid) {
    const ts = Math.floor(Date.now() / 1000);
    const hash = await sha1Hex(`${ts} ${sapisid} https://gemini.google.com`);
    headers["Authorization"] = `SAPISIDHASH ${ts}_${hash}`;
  }

  return headers;
}

function resolveModel(modelName, defaultModel = "gemini-3.6-flash") {
  let thinkOverride = null;
  if (modelName && modelName.includes("@think=")) {
    const parts = modelName.split("@think=");
    modelName = parts[0];
    const val = parseInt(parts[1], 10);
    if (!isNaN(val)) thinkOverride = val;
  }

  let cfg = MODELS[modelName];
  if (!cfg) {
    modelName = defaultModel;
    cfg = MODELS[defaultModel] || MODELS["gemini-3.6-flash"];
  }

  return {
    modelName,
    modeId: cfg.mode,
    thinkMode: thinkOverride !== null ? thinkOverride : cfg.think,
    extra: cfg.extra,
  };
}

function buildPayload(prompt, modeId, thinkMode, fileRefs, extraFields, cfg) {
  const inner = new Array(102).fill(null);
  if (fileRefs && fileRefs.length > 0) {
    const refs = fileRefs.map((ref) => [null, null, ref]);
    inner[0] = [prompt, 0, null, refs, null, null, 0];
  } else {
    inner[0] = [prompt, 0, null, null, null, null, 0];
  }

  inner[1] = ["en"];
  inner[2] = ["", "", "", null, null, null, null, null, null, ""];
  inner[6] = [0];
  inner[7] = 1;
  inner[10] = 1;
  inner[11] = 0;
  inner[17] = [[thinkMode]];
  inner[18] = 0;
  inner[27] = 1;
  inner[30] = [4];
  inner[41] = [2];
  inner[53] = 0;
  inner[59] = crypto.randomUUID();
  inner[61] = [];
  inner[68] = 1;
  inner[79] = modeId;

  if (extraFields) {
    for (const [k, v] of Object.entries(extraFields)) {
      inner[parseInt(k, 10)] = v;
    }
  }

  const outer = [null, JSON.stringify(inner)];
  const params = new URLSearchParams();
  params.set("f.req", JSON.stringify(outer));
  if (cfg.xsrfToken) params.set("at", cfg.xsrfToken);

  return params.toString();
}

function cleanText(text, strip = true) {
  if (!text) return "";
  text = text.replace(/```(?:python|javascript|text)\?code_(?:reference|stdout)&code_event_index=\d+\n.*?```\n?/gs, "");
  text = text.replace(/http:\/\/googleusercontent\.com\/card_content\/\d+\n?/g, "");
  return strip ? text.trim() : text;
}

function extractTextsFromLine(line) {
  if (!line.includes('"wrb.fr"') || line.length < 200) return [];
  try {
    const arr = JSON.parse(line);
    const innerStr = arr?.[0]?.[2];
    if (!innerStr || innerStr.length < 50) return [];
    const inner = JSON.parse(innerStr);
    if (!Array.isArray(inner) || inner.length <= 4 || !inner[4]) return [];

    const texts = [];
    for (const part of inner[4]) {
      if (Array.isArray(part) && part.length > 1 && Array.isArray(part[1])) {
        for (const t of part[1]) {
          if (typeof t === "string" && t) texts.push(t);
        }
      }
    }
    return texts;
  } catch {
    return [];
  }
}

function extractResponseText(raw) {
  if (raw.includes("BardErrorInfo")) {
    const m = raw.match(/BardErrorInfo\s*\[(\d+)\]/);
    throw new Error(`Gemini upstream rejected request: BardErrorInfo [${m ? m[1] : "unknown"}]`);
  }
  let lastText = "";
  for (const line of raw.split("\n")) {
    for (const t of extractTextsFromLine(line)) {
      if (t.length > lastText.length) lastText = t;
    }
  }
  return cleanText(lastText, true);
}

function messagesToPrompt(messages, tools, toolChoice) {
  const parts = [];
  if (tools && tools.length > 0 && toolChoice !== "none") {
    const toolDefs = tools.map((t) => {
      const fn = t.type === "function" && t.function ? t.function : t;
      return {
        name: fn.name || t.name || "",
        description: fn.description || t.description || "",
        parameters: fn.parameters || t.parameters || { type: "object", properties: {} },
      };
    });

    let constraint = "";
    if (toolChoice === "required") {
      constraint = "\n\nIMPORTANT: You MUST call at least one tool. Do not respond with text only.";
    } else if (typeof toolChoice === "object" && toolChoice?.function?.name) {
      constraint = `\n\nIMPORTANT: You MUST call the tool "${toolChoice.function.name}". Do not call other tools.`;
    }

    parts.push(
      `# Tool Use\n\nYou can call the following tools. Call format:\n\`\`\`tool_call\n{"name": "func_name", "arguments": {...}}\n\`\`\`\nWhen calling tools, output ONLY the tool_call block(s).\n\nAvailable tools:\n${JSON.stringify(toolDefs, null, 2)}${constraint}`
    );
  }

  for (const msg of messages || []) {
    const role = msg.role || "user";
    let content = msg.content || "";
    if (Array.isArray(content)) {
      content = content.map((c) => (c.text || "")).join(" ");
    }

    if (role === "system") {
      parts.push(`[System instruction]: ${content}`);
    } else if (role === "assistant") {
      if (msg.tool_calls && msg.tool_calls.length > 0) {
        const tcStrs = msg.tool_calls.map((tc) => {
          const fn = tc.function || {};
          return `\`\`\`tool_call\n{"name": "${fn.name}", "arguments": ${fn.arguments || "{}"}}\n\`\`\``;
        });
        parts.push(`[Assistant]: ${content || ""}\n${tcStrs.join("\n")}`);
      } else {
        parts.push(`[Assistant]: ${content}`);
      }
    } else if (role === "tool") {
      parts.push(`[Tool result for ${msg.name || ""}]: ${content}`);
    } else {
      if (content) parts.push(content);
    }
  }

  return parts.join("\n\n");
}

function parseToolCalls(text) {
  const toolCalls = [];
  const pattern = /```tool_call\s*\n(.*?)\n```/gs;
  const clean = text.replace(pattern, (_, jsonStr) => {
    try {
      const data = JSON.parse(jsonStr.trim());
      if (data.name) {
        toolCalls.push({
          id: `call_${crypto.randomUUID().slice(0, 8)}`,
          type: "function",
          function: {
            name: data.name,
            arguments: typeof data.arguments === "string" ? data.arguments : JSON.stringify(data.arguments || {}),
          },
        });
      }
    } catch { }
    return "";
  });
  return { cleanText: clean.trim(), toolCalls };
}

function googleContentsToPrompt(req) {
  const parts = [];
  const fcMode = req.toolConfig?.functionCallingConfig?.mode || "AUTO";
  const tools = req.tools || [];
  const toolDefs = [];

  if (tools.length > 0 && fcMode !== "NONE") {
    for (const tg of tools) {
      for (const fn of tg.functionDeclarations || []) {
        toolDefs.push({
          name: fn.name || "",
          description: fn.description || "",
          parameters: fn.parameters || fn.parametersJsonSchema || {},
        });
      }
    }
  }

  if (req.systemInstruction?.parts) {
    const sysText = req.systemInstruction.parts.map((p) => p.text || "").join(" ").trim();
    if (sysText) parts.push(sysText);
  }

  if (toolDefs.length > 0) {
    parts.push(
      `# Tool Use\n\nYou can call tools using format:\n\`\`\`function_call\n{"name": "<tool_name>", "args": {<arguments>}}\n\`\`\`\nAvailable tools:\n${JSON.stringify(toolDefs, null, 2)}`
    );
  }

  for (const c of req.contents || []) {
    const role = c.role || "user";
    const textParts = [];
    for (const p of c.parts || []) {
      if (p.text) textParts.push(p.text);
      else if (p.functionCall) {
        textParts.push(`\`\`\`function_call\n${JSON.stringify({ name: p.functionCall.name, args: p.functionCall.args || {} })}\n\`\`\``);
      } else if (p.functionResponse) {
        textParts.push(`[Tool result for ${p.functionResponse.name || ""}]: ${JSON.stringify(p.functionResponse.response || {})}`);
      }
    }
    const t = textParts.join("\n");
    if (role === "model") parts.push(`[Assistant]: ${t}`);
    else if (t) parts.push(t);
  }

  return parts.join("\n\n");
}

function parseGoogleFunctionCalls(text) {
  const functionCalls = [];
  const patterns = [/```function_call\s*\n(.*?)\n```/gs, /(?:^|\n)function_call\s*\n(\{[^`]*?\})/gs];
  let clean = text;

  for (const pat of patterns) {
    clean = clean.replace(pat, (_, jsonStr) => {
      try {
        const data = JSON.parse(jsonStr.trim());
        if (data.name) {
          functionCalls.push({ name: data.name, args: data.args || data.arguments || {} });
        }
      } catch { }
      return "";
    });
  }

  clean = clean.trim();
  if (functionCalls.length === 0 && clean.startsWith("{")) {
    try {
      const data = JSON.parse(clean);
      if (data.name && (data.args || data.arguments)) {
        functionCalls.push({ name: data.name, args: data.args || data.arguments || {} });
        clean = "";
      }
    } catch { }
  }

  return { cleanText: clean, functionCalls };
}

function jsonResponse(data, status = 200, headers = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Access-Control-Allow-Origin": "*",
      ...headers,
    },
  });
}

function authorize(request, cfg) {
  if (!cfg.apiKeys || cfg.apiKeys.length === 0) return true;
  const auth = request.headers.get("Authorization") || "";
  if (auth.startsWith("Bearer ") && cfg.apiKeys.includes(auth.slice(7))) return true;

  for (const h of ["x-api-key", "x-goog-api-key"]) {
    const val = request.headers.get(h);
    if (val && cfg.apiKeys.includes(val)) return true;
  }

  const url = new URL(request.url);
  const keyParam = url.searchParams.get("key");
  if (keyParam && cfg.apiKeys.includes(keyParam)) return true;

  return false;
}

export default {
  async fetch(request, env, ctx) {
    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
          "Access-Control-Allow-Headers": "*",
        },
      });
    }

    const cfg = getRequestConfig(env);
    const url = new URL(request.url);
    const ip = request.headers.get("cf-connecting-ip") || "127.0.0.1";

    if (!checkRateLimit(ip, cfg)) {
      return jsonResponse({ error: { message: "rate limit exceeded" } }, 429);
    }

    if ((url.pathname.startsWith("/v1") || url.pathname.startsWith("/v1beta")) && !authorize(request, cfg)) {
      return jsonResponse({ error: { message: "invalid api key" } }, 401);
    }

    if (request.method === "GET") {
      if (url.pathname === "/") {
        return jsonResponse({ status: "ok", version: "1.0.0", models: Object.keys(MODELS) });
      }
      if (url.pathname === "/v1/models") {
        return jsonResponse({
          object: "list",
          data: Object.entries(MODELS).map(([id, m]) => ({
            id,
            object: "model",
            created: 1700000000,
            owned_by: "google",
            description: m.desc,
          })),
        });
      }
      if (url.pathname.startsWith("/v1beta/models")) {
        return jsonResponse({
          models: Object.entries(MODELS).map(([id, m]) => ({
            name: `models/${id}`,
            displayName: id,
            description: m.desc,
            supportedGenerationMethods: ["generateContent", "streamGenerateContent"],
          })),
        });
      }
    }

    if (request.method === "POST") {
      let body;
      try {
        body = await request.json();
      } catch {
        return jsonResponse({ error: { message: "invalid JSON body" } }, 400);
      }

      if (url.pathname === "/v1/chat/completions") {
        return handleChat(body, cfg);
      }
      if (url.pathname === "/v1/responses") {
        return handleResponses(body, cfg);
      }
      if (url.pathname.includes(":streamGenerateContent")) {
        return handleGoogleGenerate(url.pathname, body, cfg, true);
      }
      if (url.pathname.includes(":generateContent")) {
        return handleGoogleGenerate(url.pathname, body, cfg, false);
      }
    }

    return jsonResponse({ error: "not found" }, 404);
  },
};

async function handleChat(req, cfg) {
  const { modelName, modeId, thinkMode, extra } = resolveModel(req.model, cfg.defaultModel);
  const prompt = messagesToPrompt(req.messages, req.tools, req.tool_choice || "auto");
  if (!prompt.trim()) return jsonResponse({ error: { message: "empty prompt" } }, 400);

  const stream = !!req.stream;
  const hasNoTools = !req.tools || req.tools.length === 0 || req.tool_choice === "none";
  const cid = `chatcmpl-${crypto.randomUUID().slice(0, 12)}`;

  const reqHeaders = await getAuthHeaders(cfg);
  const reqBody = buildPayload(prompt, modeId, thinkMode, null, extra, cfg);
  const reqId = (Date.now() + Math.floor(Math.random() * 1000)) % 1000000;
  const targetURL = `https://gemini.google.com${cfg.authUser ? `/u/${cfg.authUser}` : ""}/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=${cfg.geminiBl}&hl=en&_reqid=${reqId}&rt=c`;

  if (stream && hasNoTools) {
    const upstreamResp = await fetch(targetURL, { method: "POST", headers: reqHeaders, body: reqBody });
    if (!upstreamResp.ok) return jsonResponse({ error: { message: `upstream error: ${upstreamResp.status}` } }, 502);

    const { readable, writable } = new TransformStream();
    const writer = writable.getWriter();
    const encoder = new TextEncoder();

    (async () => {
      let emittedRaw = "";
      const reader = upstreamResp.body.getReader();
      let buf = "";

      const firstChunk = {
        id: cid,
        object: "chat.completion.chunk",
        created: Math.floor(Date.now() / 1000),
        model: modelName,
        choices: [{ index: 0, delta: { role: "assistant" }, finish_reason: null }],
      };
      await writer.write(encoder.encode(`data: ${JSON.stringify(firstChunk)}\n\n`));

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += new TextDecoder().decode(value);

        while (buf.includes("\n")) {
          const idx = buf.indexOf("\n");
          const line = buf.slice(0, idx).trim();
          buf = buf.slice(idx + 1);

          for (const t of extractTextsFromLine(line)) {
            if (t === emittedRaw || emittedRaw.startsWith(t)) continue;
            const delta = cleanText(t.startsWith(emittedRaw) ? t.slice(emittedRaw.length) : t, false);
            emittedRaw = t;
            if (delta) {
              const chunk = {
                id: cid,
                object: "chat.completion.chunk",
                created: Math.floor(Date.now() / 1000),
                model: modelName,
                choices: [{ index: 0, delta: { content: delta }, finish_reason: null }],
              };
              await writer.write(encoder.encode(`data: ${JSON.stringify(chunk)}\n\n`));
            }
          }
        }
      }

      const endChunk = {
        id: cid,
        object: "chat.completion.chunk",
        created: Math.floor(Date.now() / 1000),
        model: modelName,
        choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
      };
      await writer.write(encoder.encode(`data: ${JSON.stringify(endChunk)}\n\ndata: [DONE]\n\n`));
      await writer.close();
    })().catch(() => writer.close());

    return new Response(readable, {
      headers: {
        "Content-Type": "text/event-stream; charset=utf-8",
        "Cache-Control": "no-cache",
        "Access-Control-Allow-Origin": "*",
      },
    });
  }

  const resp = await fetch(targetURL, { method: "POST", headers: reqHeaders, body: reqBody });
  if (!resp.ok) return jsonResponse({ error: { message: `upstream error ${resp.status}` } }, 502);

  const raw = await resp.text();
  let text = extractResponseText(raw);
  let toolCalls = null;

  if (req.tools && text && req.tool_choice !== "none") {
    const parsed = parseToolCalls(text);
    text = parsed.cleanText;
    toolCalls = parsed.toolCalls.length > 0 ? parsed.toolCalls : null;
  }

  const msg = { role: "assistant", content: text || null };
  if (toolCalls) msg.tool_calls = toolCalls;
  const finish = toolCalls ? "tool_calls" : "stop";

  if (stream) {
    const chunk = {
      id: cid,
      object: "chat.completion.chunk",
      created: Math.floor(Date.now() / 1000),
      model: modelName,
      choices: [{ index: 0, delta: msg, finish_reason: finish }],
    };
    return new Response(`data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n`, {
      headers: {
        "Content-Type": "text/event-stream; charset=utf-8",
        "Cache-Control": "no-cache",
        "Access-Control-Allow-Origin": "*",
      },
    });
  }

  return jsonResponse({
    id: cid,
    object: "chat.completion",
    created: Math.floor(Date.now() / 1000),
    model: modelName,
    choices: [{ index: 0, message: msg, finish_reason: finish }],
    usage: {
      prompt_tokens: Math.floor(prompt.length / 4),
      completion_tokens: Math.floor((text || "").length / 4),
      total_tokens: Math.floor((prompt.length + (text || "").length) / 4),
    },
  });
}

async function handleResponses(req, cfg) {
  const { modelName, modeId, thinkMode, extra } = resolveModel(req.model, cfg.defaultModel);
  const messages = [];
  if (req.instructions) messages.push({ role: "system", content: req.instructions });

  if (typeof req.input === "string") {
    messages.push({ role: "user", content: req.input });
  } else if (Array.isArray(req.input)) {
    for (const item of req.input) {
      if (typeof item === "string") messages.push({ role: "user", content: item });
      else if (item.type === "function_call_output") {
        messages.push({ role: "tool", name: item.name, content: item.output });
      } else messages.push({ role: item.role || "user", content: item.content || "" });
    }
  }

  const prompt = messagesToPrompt(messages, req.tools, req.tool_choice || "auto");
  const reqHeaders = await getAuthHeaders(cfg);
  const reqBody = buildPayload(prompt, modeId, thinkMode, null, extra, cfg);
  const reqId = (Date.now() + Math.floor(Math.random() * 1000)) % 1000000;
  const targetURL = `https://gemini.google.com${cfg.authUser ? `/u/${cfg.authUser}` : ""}/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=${cfg.geminiBl}&hl=en&_reqid=${reqId}&rt=c`;

  const resp = await fetch(targetURL, { method: "POST", headers: reqHeaders, body: reqBody });
  if (!resp.ok) return jsonResponse({ error: { message: `upstream error ${resp.status}` } }, 502);

  const raw = await resp.text();
  let text = extractResponseText(raw);
  let toolCalls = null;

  if (req.tools && text && req.tool_choice !== "none") {
    const parsed = parseToolCalls(text);
    text = parsed.cleanText;
    toolCalls = parsed.toolCalls.length > 0 ? parsed.toolCalls : null;
  }

  const rid = `resp_${crypto.randomUUID().slice(0, 16)}`;
  const mid = `msg_${crypto.randomUUID().slice(0, 12)}`;
  const output = [];

  if (toolCalls) {
    for (const tc of toolCalls) {
      output.push({
        type: "function_call",
        id: tc.id,
        call_id: tc.id,
        name: tc.function.name,
        arguments: tc.function.arguments,
        status: "completed",
      });
    }
  }

  if (text || !toolCalls) {
    output.push({
      type: "message",
      id: mid,
      role: "assistant",
      status: "completed",
      content: [{ type: "output_text", text: text || "", annotations: [] }],
    });
  }

  return jsonResponse({
    id: rid,
    object: "response",
    created_at: Math.floor(Date.now() / 1000),
    status: "completed",
    model: modelName,
    output,
    usage: {
      input_tokens: Math.floor(prompt.length / 4),
      output_tokens: Math.floor((text || "").length / 4),
      total_tokens: Math.floor((prompt.length + (text || "").length) / 4),
    },
  });
}

async function handleGoogleGenerate(pathname, req, cfg, stream) {
  const m = pathname.match(/\/v1beta\/models\/([^:?]+)/);
  const rawModel = m ? m[1] : cfg.defaultModel;
  const { modelName, modeId, thinkMode, extra } = resolveModel(rawModel, cfg.defaultModel);

  const prompt = googleContentsToPrompt(req);
  const reqHeaders = await getAuthHeaders(cfg);
  const reqBody = buildPayload(prompt, modeId, thinkMode, null, extra, cfg);
  const reqId = (Date.now() + Math.floor(Math.random() * 1000)) % 1000000;
  const targetURL = `https://gemini.google.com${cfg.authUser ? `/u/${cfg.authUser}` : ""}/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=${cfg.geminiBl}&hl=en&_reqid=${reqId}&rt=c`;

  const resp = await fetch(targetURL, { method: "POST", headers: reqHeaders, body: reqBody });
  if (!resp.ok) return jsonResponse({ error: { message: `upstream error ${resp.status}` } }, 502);

  const raw = await resp.text();
  const text = extractResponseText(raw);
  const { cleanText: textClean, functionCalls } = parseGoogleFunctionCalls(text);

  const parts = [];
  if (functionCalls.length > 0) {
    if (textClean) parts.push({ text: textClean });
    for (const fc of functionCalls) parts.push({ functionCall: { name: fc.name, args: fc.args } });
  } else {
    parts.push({ text: text || "I apologize, but I was unable to generate a response." });
  }

  const responseObj = {
    candidates: [{ content: { parts, role: "model" }, finishReason: "STOP", index: 0 }],
    usageMetadata: {
      promptTokenCount: Math.floor(prompt.length / 4),
      candidatesTokenCount: Math.floor((text || "").length / 4),
      totalTokenCount: Math.floor((prompt.length + (text || "").length) / 4),
    },
    modelVersion: modelName,
  };

  if (stream) {
    return new Response(`data: ${JSON.stringify(responseObj)}\n\n`, {
      headers: {
        "Content-Type": "text/event-stream; charset=utf-8",
        "Cache-Control": "no-cache",
        "Access-Control-Allow-Origin": "*",
      },
    });
  }

  return jsonResponse(responseObj);
}
