document.addEventListener('DOMContentLoaded', () => {
  // Initialize Lucide Icons
  if (window.lucide) {
    window.lucide.createIcons();
  }

  // Playground Simulation / Live Test
  const promptInput = document.getElementById('playground-prompt');
  const runBtn = document.getElementById('playground-run-btn');
  const outputEl = document.getElementById('playground-output');
  const modelSelect = document.getElementById('playground-model');
  const thinkSelect = document.getElementById('playground-think');

  const DEMO_RESPONSES = {
    "Explain quantum computing in one sentence.": "Quantum computing harnesses quantum mechanical phenomena like superposition and entanglement to solve complex mathematical calculations exponentially faster than classical computers.",
    "Write a short poem about coding in Go.": "Goroutines hum through threads of light,\nChannels flow like rivers bright.\nCompiled fast, with memory lean,\nThe cleanest code you've ever seen.",
    "What is the square root of 144?": "The square root of 144 is 12 (since 12 × 12 = 144).",
    "Default": "Hello! I am Gemini, streaming through the ultra-fast gemini-shim proxy. How can I assist you today?"
  };

  async function streamTypewriter(text) {
    outputEl.innerHTML = '';
    const cursor = document.createElement('span');
    cursor.className = 'cursor-blink';
    outputEl.appendChild(cursor);

    const words = text.split(' ');
    for (let i = 0; i < words.length; i++) {
      cursor.before((i > 0 ? ' ' : '') + words[i]);
      await new Promise(r => setTimeout(r, 20 + Math.random() * 18));
    }
  }

  runBtn.addEventListener('click', async () => {
    const prompt = promptInput.value.trim();
    if (!prompt) return;

    runBtn.disabled = true;
    runBtn.innerHTML = '<i data-lucide="loader" class="lucide-spin"></i> Generating...';
    if (window.lucide) window.lucide.createIcons();
    outputEl.textContent = '';

    // Check if user has local gemini-shim running on localhost:8081
    try {
      let targetModel = modelSelect.value;
      if (thinkSelect.value === 'deep') {
        targetModel += '@think=0';
      }

      const res = await fetch('http://localhost:8081/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: targetModel,
          messages: [{ role: 'user', content: prompt }],
          stream: false
        })
      });

      if (res.ok) {
        const data = await res.json();
        const content = data.choices?.[0]?.message?.content || JSON.stringify(data, null, 2);
        await streamTypewriter(content);
        runBtn.disabled = false;
        runBtn.innerHTML = '<i data-lucide="play"></i> Run Test';
        if (window.lucide) window.lucide.createIcons();
        return;
      }
    } catch (e) {
      // Local server not running or CORS blocked -> fallback to simulated demo
    }

    const responseText = DEMO_RESPONSES[prompt] || DEMO_RESPONSES["Default"];
    await streamTypewriter(responseText);
    runBtn.disabled = false;
    runBtn.innerHTML = '<i data-lucide="play"></i> Run Test';
    if (window.lucide) window.lucide.createIcons();
  });

  // Code Tab Switcher
  const tabBtns = document.querySelectorAll('.tab-btn');
  const codeBlocks = document.querySelectorAll('.code-pane');

  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => b.classList.remove('active'));
      codeBlocks.forEach(c => c.style.display = 'none');

      btn.classList.add('active');
      const target = document.getElementById(btn.dataset.target);
      if (target) target.style.display = 'block';
    });
  });

  // Quick Copy Helper
  window.copySnippet = function(elementId, btn) {
    const el = document.getElementById(elementId);
    if (!el) return;
    const text = el.innerText || el.textContent;
    navigator.clipboard.writeText(text.trim());
    const originalHTML = btn.innerHTML;
    btn.innerHTML = '<i data-lucide="check"></i> Copied';
    if (window.lucide) window.lucide.createIcons();
    setTimeout(() => {
      btn.innerHTML = originalHTML;
      if (window.lucide) window.lucide.createIcons();
    }, 2000);
  };
});
