import { GeminiShim } from "../src/index.ts";

const ai = new GeminiShim({
  baseURL: "http://127.0.0.1:8081/v1",
  defaultModel: "gemini-3.7-flash",
});

async function main() {
  const chat = ai.createChat({
    systemInstruction: "You are a witty, concise AI assistant.",
  });

  console.log("User: Hello, I live in Tokyo.");
  const r1 = await chat.sendMessage("Hello, I live in Tokyo.");
  console.log(`Assistant: ${r1.text}\n`);

  console.log("User: What should I eat for lunch nearby?");
  const r2 = await chat.sendMessage("What should I eat for lunch nearby?");
  console.log(`Assistant: ${r2.text}\n`);
}

main().catch(console.error);
