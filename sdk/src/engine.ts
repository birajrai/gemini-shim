import type { ChatMessage, GeminiModel, Tool, ToolCall } from "./types";

let cachedBL = "boq_assistant-bard-web-server_20260827.05_p0";

const MODEL_MAP: Record<string, { mode: number; think: number; extra?: Record<number, number>; desc: string }> = {
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
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
];

async function sha1Hex(str: string): Promise<string> {
  if (typeof crypto !== "undefined" && crypto.subtle) {
    const msgUint8 = new TextEncoder().encode(str);
    const hashBuffer = await crypto.subtle.digest("SHA-1", msgUint8);
    return Array.from(new Uint8Array(hashBuffer))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }
  return "";
}

/**
 * Dynamically queries Gemini Web to fetch the current build label if 405 error occurs.
 */
export async function fetchLatestBuildLabel(customFetch: typeof fetch = fetch): Promise<string> {
  try {
    const res = await customFetch("https://gemini.google.com/app", {
      headers: {
        "User-Agent": USER_AGENTS[0],
      },
    });
    if (res.ok) {
      const text = await res.text();
      const m = text.match(/"cfb2h":"(boq_assistant-bard-web-server_[^"]+)"/);
      if (m && m[1]) {
        cachedBL = m[1];
        return m[1];
      }
    }
  } catch {
    // Fallback to cached
  }
  return cachedBL;
}

export function resolveModel(modelName?: string, defaultModel = "gemini-3.7-flash") {
  let thinkOverride: number | null = null;
  let targetModel = modelName || defaultModel;

  if (targetModel.includes("@think=")) {
    const parts = targetModel.split("@think=");
    targetModel = parts[0];
    const val = parseInt(parts[1], 10);
    if (!isNaN(val)) thinkOverride = val;
  }

  let cfg = MODEL_MAP[targetModel];
  if (!cfg) {
    targetModel = defaultModel;
    cfg = MODEL_MAP[defaultModel] || MODEL_MAP["gemini-3.7-flash"];
  }

  return {
    modelName: targetModel,
    modeId: cfg.mode,
    thinkMode: thinkOverride !== null ? thinkOverride : cfg.think,
    extra: cfg.extra,
  };
}

export function messagesToPrompt(
  messages?: ChatMessage[],
  tools?: Tool[],
  toolChoice: string | { type: string; function: { name: string } } = "auto"
): string {
  const parts: string[] = [];

  if (tools && tools.length > 0 && toolChoice !== "none") {
    const toolDefs = tools.map((t) => {
      const fn = (t.type === "function" && t.function ? t.function : t) as any;
      return {
        name: fn.name || "",
        description: fn.description || "",
        parameters: fn.parameters || { type: "object", properties: {} },
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
    const content = typeof msg.content === "string" ? msg.content : JSON.stringify(msg.content);

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

export function buildDirectPayload(
  prompt: string,
  modeId: number,
  thinkMode: number,
  extraFields?: Record<number, number>,
  xsrfToken?: string
): string {
  const inner: any[] = new Array(102).fill(null);
  inner[0] = [prompt, 0, null, null, null, null, 0];
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
  inner[59] = typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : "uuid-" + Math.random();
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
  if (xsrfToken) params.set("at", xsrfToken);

  return params.toString();
}

export async function buildDirectHeaders(cookie?: string, authUser?: string): Promise<Record<string, string>> {
  const ua = USER_AGENTS[Math.floor(Math.random() * USER_AGENTS.length)];
  const accountPrefix = authUser ? `/u/${authUser}` : "";

  const headers: Record<string, string> = {
    "Content-Type": "application/x-www-form-urlencoded",
    Origin: "https://gemini.google.com",
    Referer: `https://gemini.google.com${accountPrefix}/app`,
    "X-Same-Domain": "1",
    "User-Agent": ua,
    "Accept-Language": "en-US,en;q=0.9",
  };

  if (authUser) {
    headers["X-Goog-AuthUser"] = String(authUser);
  }

  if (cookie) {
    headers["Cookie"] = cookie;
    const m = cookie.match(/SAPISID=([^;]+)/);
    if (m) {
      const sapisid = m[1].trim();
      const ts = Math.floor(Date.now() / 1000);
      const hash = await sha1Hex(`${ts} ${sapisid} https://gemini.google.com`);
      if (hash) headers["Authorization"] = `SAPISIDHASH ${ts}_${hash}`;
    }
  }

  return headers;
}

export function cleanText(text?: string, strip = true): string {
  if (!text) return "";
  let clean = text.replace(/```(?:python|javascript|text)\?code_(?:reference|stdout)&code_event_index=\d+\n.*?```\n?/gs, "");
  clean = clean.replace(/http:\/\/googleusercontent\.com\/card_content\/\d+\n?/g, "");
  return strip ? clean.trim() : clean;
}

export function extractTextsFromLine(line: string): string[] {
  if (!line.includes('"wrb.fr"') || line.length < 200) return [];
  try {
    const arr = JSON.parse(line);
    const innerStr = arr?.[0]?.[2];
    if (!innerStr || innerStr.length < 50) return [];
    const inner = JSON.parse(innerStr);
    if (!Array.isArray(inner) || inner.length <= 4 || !inner[4]) return [];

    const texts: string[] = [];
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

export function extractResponseText(raw: string): string {
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

export function parseToolCalls(text: string): { cleanText: string; toolCalls: ToolCall[] } {
  const toolCalls: ToolCall[] = [];
  const pattern = /```tool_call\s*\n(.*?)\n```/gs;
  const clean = text.replace(pattern, (_, jsonStr) => {
    try {
      const data = JSON.parse(jsonStr.trim());
      if (data.name) {
        toolCalls.push({
          id: `call_${Math.random().toString(36).slice(2, 10)}`,
          type: "function",
          function: {
            name: data.name,
            arguments: typeof data.arguments === "string" ? data.arguments : JSON.stringify(data.arguments || {}),
          },
        });
      }
    } catch {}
    return "";
  });
  return { cleanText: clean.trim(), toolCalls };
}

/**
 * Performs a direct Native RPC request to Google Gemini with automatic 405 error recovery.
 */
export async function executeDirectGemini(
  prompt: string,
  model: GeminiModel = "gemini-3.7-flash",
  options: {
    cookie?: string;
    authUser?: string;
    xsrfToken?: string;
    fetch?: typeof fetch;
    signal?: AbortSignal;
  } = {}
): Promise<Response> {
  const customFetch = options.fetch || fetch;
  const { modeId, thinkMode, extra } = resolveModel(model);
  const reqHeaders = await buildDirectHeaders(options.cookie, options.authUser);
  const reqBody = buildDirectPayload(prompt, modeId, thinkMode, extra, options.xsrfToken);

  const getURL = (bl: string) => {
    const accountPrefix = options.authUser ? `/u/${options.authUser}` : "";
    const reqId = (Date.now() + Math.floor(Math.random() * 1000)) % 1000000;
    return `https://gemini.google.com${accountPrefix}/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=${bl}&hl=en&_reqid=${reqId}&rt=c`;
  };

  let bl = cachedBL;
  let resp = await customFetch(getURL(bl), {
    method: "POST",
    headers: reqHeaders,
    body: reqBody,
    signal: options.signal,
  });

  if (resp.status === 405) {
    const freshBL = await fetchLatestBuildLabel(customFetch);
    if (freshBL && freshBL !== bl) {
      resp = await customFetch(getURL(freshBL), {
        method: "POST",
        headers: reqHeaders,
        body: reqBody,
        signal: options.signal,
      });
    }
  }

  return resp;
}
