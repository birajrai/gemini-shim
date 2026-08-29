const CORE_REQUIRED = ["SAPISID"];
const SESSION_ALTERNATIVES = ["__Secure-1PSID", "__Secure-3PSID", "SID"];

const EXPORT_ORDER = [
  "SID",
  "HSID",
  "SSID",
  "APISID",
  "SAPISID",
  "LSID",
  "OSID",
  "SIDCC",
  "AEC",
  "NID",
  "COMPASS",
  "__Secure-1PAPISID",
  "__Secure-1PSID",
  "__Secure-1PSIDTS",
  "__Secure-1PSIDCC",
  "__Secure-1PSIDRTS",
  "__Secure-3PAPISID",
  "__Secure-3PSID",
  "__Secure-3PSIDTS",
  "__Secure-3PSIDCC",
  "__Secure-3PSIDRTS",
  "__Secure-OSID",
  "__Host-1PLSID",
  "__Host-3PLSID"
];

const LOOKUP_URLS = [
  "https://gemini.google.com/app",
  "https://accounts.google.com/",
  "https://www.google.com/",
  "https://google.com/"
];

const statusEl = document.getElementById("status-box");
const openBtn = document.getElementById("open");
const inspectBtn = document.getElementById("inspect");
const exportJsonBtn = document.getElementById("export-json");
const copyCfBtn = document.getElementById("copy-cf");

function setStatus(msg, kind = "") {
  statusEl.textContent = msg;
  statusEl.className = kind ? `status-${kind}` : "";
}

function normalizeDomain(domain = "") {
  return domain.replace(/^\./, "").toLowerCase();
}

function isGoogleCookie(cookie) {
  const domain = normalizeDomain(cookie.domain);
  return domain === "google.com" || domain.endsWith(".google.com");
}

function cookieKey(cookie) {
  const partition = cookie.partitionKey ? JSON.stringify(cookie.partitionKey) : "";
  return [
    cookie.storeId || "",
    cookie.name,
    cookie.domain,
    cookie.path,
    partition
  ].join("|");
}

function scoreCookie(cookie) {
  const domain = (cookie.domain || "").toLowerCase();
  let score = 0;

  if (domain === ".google.com") score += 120;
  else if (domain === "google.com") score += 110;
  else if (domain === ".gemini.google.com") score += 100;
  else if (domain === "gemini.google.com") score += 95;
  else if (domain === ".accounts.google.com") score += 80;
  else if (domain === "accounts.google.com") score += 75;
  else if (domain.endsWith(".google.com")) score += 40;

  if (cookie.path === "/") score += 10;
  if (cookie.secure) score += 3;
  if (cookie.httpOnly) score += 2;
  if (!cookie.partitionKey) score += 2;
  if (!cookie.session) score += 1;

  return score;
}

async function readGoogleCookies() {
  const stores = await chrome.cookies.getAllCookieStores();
  const deduped = new Map();

  for (const store of stores) {
    const queries = [
      chrome.cookies.getAll({ storeId: store.id }),
      ...LOOKUP_URLS.map((url) => chrome.cookies.getAll({ storeId: store.id, url }))
    ];

    const results = await Promise.allSettled(queries);
    for (const result of results) {
      if (result.status !== "fulfilled" || !Array.isArray(result.value)) {
        continue;
      }

      for (const cookie of result.value) {
        if (!isGoogleCookie(cookie)) continue;
        const key = cookieKey(cookie);
        const existing = deduped.get(key);
        if (!existing || scoreCookie(cookie) > scoreCookie(existing)) {
          deduped.set(key, cookie);
        }
      }
    }
  }

  return Array.from(deduped.values());
}

function chooseBestCookies(cookies) {
  const chosen = new Map();
  for (const cookie of cookies) {
    const current = chosen.get(cookie.name);
    if (!current || scoreCookie(cookie) > scoreCookie(current)) {
      chosen.set(cookie.name, cookie);
    }
  }
  return chosen;
}

function formatCookieString(chosenMap) {
  const orderedNames = [
    ...EXPORT_ORDER.filter((name) => chosenMap.has(name)),
    ...Array.from(chosenMap.keys())
      .filter((name) => !EXPORT_ORDER.includes(name))
      .sort((a, b) => a.localeCompare(b))
  ];

  return orderedNames
    .map((name) => `${name}=${chosenMap.get(name).value}`)
    .join("; ");
}

async function getGeminiTab() {
  const tabs = await chrome.tabs.query({
    url: [
      "https://gemini.google.com/*",
      "https://gemini.google.com/app*"
    ]
  });

  return tabs.find((tab) => tab.url && tab.url.includes("gemini.google.com")) || null;
}

async function extractPageMetadata(tabId) {
  try {
    const [result] = await chrome.scripting.executeScript({
      target: { tabId },
      world: "MAIN",
      func: () => {
        const out = {
          url: window.location.href,
          xsrf_token: null,
          gemini_bl: null,
          auth_user: null
        };

        const authMatch = window.location.pathname.match(/\/u\/(\d+)/);
        if (authMatch) out.auth_user = authMatch[1];

        const wiz = window.WIZ_global_data;
        if (wiz && typeof wiz === "object") {
          out.xsrf_token = wiz.SNlM0e || null;
          out.gemini_bl = wiz.cfb2h || null;
          if (out.auth_user === null && wiz.oPEP7c !== undefined) {
            out.auth_user = String(wiz.oPEP7c);
          }
        }

        return out;
      }
    });

    return result ? result.result : null;
  } catch (e) {
    return null;
  }
}

async function collectSessionData() {
  const rawCookies = await readGoogleCookies();
  const bestCookies = chooseBestCookies(rawCookies);
  const cookieStr = formatCookieString(bestCookies);
  const sapisid = bestCookies.get("SAPISID") ? bestCookies.get("SAPISID").value : "";

  const tab = await getGeminiTab();
  let meta = null;
  if (tab && tab.id) {
    meta = await extractPageMetadata(tab.id);
  }

  return {
    rawCookies,
    bestCookies,
    cookieStr,
    sapisid,
    tab,
    meta
  };
}

inspectBtn.addEventListener("click", async () => {
  inspectBtn.disabled = true;
  setStatus("Inspecting Google session...");

  try {
    const data = await collectSessionData();
    const hasSapisid = !!data.sapisid;
    const hasSession = SESSION_ALTERNATIVES.some((n) => data.bestCookies.has(n));

    let report = [];
    report.push(`• Total Google cookies: ${data.bestCookies.size}`);
    report.push(`• SAPISID: ${hasSapisid ? "✓ Found" : "✗ Missing"}`);
    report.push(`• Session token: ${hasSession ? "✓ Found" : "✗ Missing"}`);

    if (data.meta) {
      report.push(`• XSRF Token (SNlM0e): ${data.meta.xsrf_token ? "✓ Present" : "None"}`);
      report.push(`• Build Tag (cfb2h): ${data.meta.gemini_bl ? data.meta.gemini_bl : "None"}`);
      if (data.meta.auth_user) {
        report.push(`• Account index: /u/${data.meta.auth_user}`);
      }
    } else {
      report.push(`• Gemini Tab: Open https://gemini.google.com/app and refresh for build metadata.`);
    }

    if (hasSapisid) {
      report.push("\nSession is valid and ready to export.");
      setStatus(report.join("\n"), "ok");
    } else {
      report.push("\nSAPISID not found. Please log in to Google Gemini first.");
      setStatus(report.join("\n"), "warn");
    }
  } catch (err) {
    setStatus(`Error: ${err.message}`, "err");
  } finally {
    inspectBtn.disabled = false;
  }
});

exportJsonBtn.addEventListener("click", async () => {
  exportJsonBtn.disabled = true;
  setStatus("Generating gemini-auth.json...");

  try {
    const data = await collectSessionData();
    if (!data.sapisid) {
      setStatus("Error: SAPISID not found. Please log in to Gemini first.", "err");
      exportJsonBtn.disabled = false;
      return;
    }

    const payload = {
      cookie: data.cookieStr,
      sapisid: data.sapisid,
      auth_user: data.meta && data.meta.auth_user ? data.meta.auth_user : null,
      xsrf_token: data.meta && data.meta.xsrf_token ? data.meta.xsrf_token : null,
      gemini_bl: data.meta && data.meta.gemini_bl ? data.meta.gemini_bl : "boq_assistant-bard-web-server_20260716.08_p0",
      exported_at: new Date().toISOString()
    };

    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);

    await chrome.downloads.download({
      url: url,
      filename: "gemini-auth.json",
      saveAs: true
    });

    setStatus("Downloaded gemini-auth.json successfully. Place it in your gemini-shim folder.", "ok");
  } catch (err) {
    setStatus(`Export failed: ${err.message}`, "err");
  } finally {
    exportJsonBtn.disabled = false;
  }
});

copyCfBtn.addEventListener("click", async () => {
  copyCfBtn.disabled = true;
  setStatus("Reading cookies for Cloudflare Workers...");

  try {
    const data = await collectSessionData();
    if (!data.cookieStr) {
      setStatus("Error: No cookies found. Please log in to Gemini first.", "err");
      copyCfBtn.disabled = false;
      return;
    }

    await navigator.clipboard.writeText(data.cookieStr);
    setStatus("Copied COOKIE_STRING to clipboard. Paste it into Cloudflare Workers Environment Variables.", "ok");
  } catch (err) {
    setStatus(`Copy failed: ${err.message}`, "err");
  } finally {
    copyCfBtn.disabled = false;
  }
});

openBtn.addEventListener("click", () => {
  chrome.tabs.create({ url: "https://gemini.google.com/app" });
});
