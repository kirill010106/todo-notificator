export function chime() {
  if (localStorage.getItem("sound_enabled") === "0") return;
  try {
    const ctx = new AudioContext();
    [523.25, 659.25, 783.99].forEach((frequency, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.frequency.value = frequency;
      gain.gain.setValueAtTime(0.05, ctx.currentTime + i * 0.1);
      gain.gain.exponentialRampToValueAtTime(
        0.001,
        ctx.currentTime + i * 0.1 + 0.3,
      );
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(ctx.currentTime + i * 0.1);
      osc.stop(ctx.currentTime + i * 0.1 + 0.3);
    });
    setTimeout(() => void ctx.close(), 800);
  } catch {
    /* Audio is optional when the browser blocks playback. */
  }
}
