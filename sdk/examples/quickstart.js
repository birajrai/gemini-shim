import { GeminiShim } from "../src/index.ts";

// 100% Native: No baseURL, no endpoint, no port, no proxy!
const ai = new GeminiShim();

async function main() {
  console.log("Asking Gemini directly (zero endpoints/ports)...");
  const response = await ai.chat("What are 3 tips for writing clean code? Answer concisely in JSON or bullet points.");
  console.log("\nResponse:\n" + response.text);
}

main().catch(console.error);
