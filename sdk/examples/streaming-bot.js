import { GeminiShim } from "../src/index.ts";

// Native Direct Mode: Zero configuration needed!
const ai = new GeminiShim();

async function main() {
  console.log("Streaming response natively (zero endpoints):\n");
  const stream = await ai.stream("Write a 3-line motivational message for software engineers.");

  for await (const chunk of stream) {
    process.stdout.write(chunk);
  }
  console.log("\n");
}

main().catch(console.error);
