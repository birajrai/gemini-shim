import { GeminiShim } from "../src/index.ts";

const ai = new GeminiShim({
  baseURL: "http://127.0.0.1:8081/v1",
  defaultModel: "gemini-3.7-flash",
});

async function main() {
  console.log("Streaming response:\n");
  const stream = await ai.stream("Write a short motivational haiku about programming.");

  for await (const chunk of stream) {
    process.stdout.write(chunk);
  }
  console.log("\n");
}

main().catch(console.error);
