(function () {
  "use strict";

  const status = document.getElementById("pow-status");
  const tokenInput = document.getElementById("pow-token");
  const nonceInput = document.getElementById("pow-nonce");
  const submit = document.getElementById("register-submit");
  if (!status || !tokenInput || !nonceInput || !submit) return;

  submit.disabled = true;
  status.textContent = "Preparing proof-of-work challenge…";

  function leadingZeroBits(bytes) {
    let bits = 0;
    for (const byte of bytes) {
      if (byte === 0) { bits += 8; continue; }
      for (let mask = 0x80; mask; mask >>= 1) {
        if (byte & mask) return bits;
        bits++;
      }
    }
    return bits;
  }

  async function solve(challenge, difficulty) {
    const encoder = new TextEncoder();
    for (let nonce = 0; ; nonce++) {
      const data = encoder.encode(challenge + ":" + nonce);
      const digest = await crypto.subtle.digest("SHA-256", data);
      if (leadingZeroBits(new Uint8Array(digest)) >= difficulty) {
        return nonce;
      }
      if (nonce % 2000 === 0) {
        status.textContent = "Solving proof-of-work… " + nonce;
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    }
  }

  (async () => {
    try {
      const response = await fetch("/api/register/challenge", {
        credentials: "same-origin",
        headers: { "Accept": "application/json" },
      });
      if (!response.ok) throw new Error("challenge request failed");
      const challenge = await response.json();
      status.textContent = "Solving proof-of-work…";
      const nonce = await solve(challenge.challenge, challenge.difficulty);
      tokenInput.value = challenge.token;
      nonceInput.value = String(nonce);
      status.textContent = "Proof-of-work complete.";
      submit.disabled = false;
    } catch (error) {
      status.textContent = "Proof-of-work failed. Reload the page and try again.";
      console.error(error);
    }
  })();
})();
