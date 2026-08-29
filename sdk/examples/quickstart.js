import { GeminiShim } from "../src/index.ts";

const ai = new GeminiShim({
  baseURL: "http://127.0.0.1:8081/v1", // or "https://gemini-shim.gauravuchil13.workers.dev/v1"
  defaultModel: "gemini-3.7-flash",
});

async function main() {
  console.log("Asking Gemini...");
  const response = await ai.chat("Explain quantum entanglement in 2 sentences.");
  console.log("\nResponse:\n" + response.text);
}

main().catch(console.error);
