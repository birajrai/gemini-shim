import { GeminiShim } from "../src/index.ts";

// Native Direct Mode: Zero configuration needed!
const ai = new GeminiShim();

async function main() {
  const chat = ai.createChat({
    systemInstruction: "You are a concise, witty AI chatbot assistant.",
  });

  console.log("User: Hi, my favorite fruit is Mango.");
  const r1 = await chat.sendMessage("Hi, my favorite fruit is Mango.");
  console.log(`Assistant: ${r1.text}\n`);

  console.log("User: What fruit did I just tell you I liked?");
  const r2 = await chat.sendMessage("What fruit did I just tell you I liked?");
  console.log(`Assistant: ${r2.text}\n`);
}

main().catch(console.error);
