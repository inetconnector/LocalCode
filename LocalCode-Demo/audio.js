// 8-Bit Retro Synthesizer using Web Audio API
class PacmanAudio {
    constructor() {
        this.ctx = null;
        this.muted = false;
        this.sirenOsc = null;
        this.sirenGain = null;
        this.isSirenPlaying = false;
        this.lastChompTick = 0;
    }

    init() {
        if (!this.ctx) {
            const AudioCtx = window.AudioContext || window.webkitAudioContext;
            if (AudioCtx) {
                this.ctx = new AudioCtx();
            }
        }
        if (this.ctx && this.ctx.state === 'suspended') {
            this.ctx.resume();
        }
    }

    toggleMute() {
        this.muted = !this.muted;
        if (this.muted && this.sirenGain) {
            this.sirenGain.gain.setValueAtTime(0, this.ctx ? this.ctx.currentTime : 0);
        }
        return this.muted;
    }

    playChomp() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const now = this.ctx.currentTime;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();

        this.lastChompTick = (this.lastChompTick + 1) % 2;
        const freq1 = this.lastChompTick === 0 ? 300 : 450;
        const freq2 = this.lastChompTick === 0 ? 150 : 200;

        osc.type = 'triangle';
        osc.frequency.setValueAtTime(freq1, now);
        osc.frequency.exponentialRampToValueAtTime(freq2, now + 0.08);

        gain.gain.setValueAtTime(0.2, now);
        gain.gain.linearRampToValueAtTime(0.01, now + 0.08);

        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.start(now);
        osc.stop(now + 0.08);
    }

    playEnergizer() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const now = this.ctx.currentTime;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();

        osc.type = 'sawtooth';
        osc.frequency.setValueAtTime(200, now);
        osc.frequency.linearRampToValueAtTime(400, now + 0.1);
        osc.frequency.linearRampToValueAtTime(200, now + 0.2);

        gain.gain.setValueAtTime(0.25, now);
        gain.gain.linearRampToValueAtTime(0.01, now + 0.2);

        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.start(now);
        osc.stop(now + 0.2);
    }

    playEatGhost() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const now = this.ctx.currentTime;
        const freqs = [350, 500, 650, 800, 950, 1100];
        freqs.forEach((f, i) => {
            const osc = this.ctx.createOscillator();
            const gain = this.ctx.createGain();
            const t = now + (i * 0.04);

            osc.type = 'square';
            osc.frequency.setValueAtTime(f, t);

            gain.gain.setValueAtTime(0.2, t);
            gain.gain.linearRampToValueAtTime(0.01, t + 0.04);

            osc.connect(gain);
            gain.connect(this.ctx.destination);

            osc.start(t);
            osc.stop(t + 0.04);
        });
    }

    playDeath() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const now = this.ctx.currentTime;
        const steps = 12;
        for (let i = 0; i < steps; i++) {
            const osc = this.ctx.createOscillator();
            const gain = this.ctx.createGain();
            const t = now + (i * 0.08);
            const freq = 600 - (i * 40);

            osc.type = 'sawtooth';
            osc.frequency.setValueAtTime(freq, t);
            osc.frequency.linearRampToValueAtTime(freq - 30, t + 0.07);

            gain.gain.setValueAtTime(0.3, t);
            gain.gain.linearRampToValueAtTime(0.01, t + 0.07);

            osc.connect(gain);
            gain.connect(this.ctx.destination);

            osc.start(t);
            osc.stop(t + 0.07);
        }
    }

    playStartIntro() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const notes = [
            { f: 493.88, d: 0.12 }, // B4
            { f: 987.77, d: 0.12 }, // B5
            { f: 739.99, d: 0.12 }, // F#5
            { f: 622.25, d: 0.12 }, // D#5
            { f: 987.77, d: 0.08 }, // B5
            { f: 739.99, d: 0.16 }, // F#5
            { f: 622.25, d: 0.20 }, // D#5
            { f: 523.25, d: 0.12 }, // C5
            { f: 1046.50, d: 0.12 },// C6
            { f: 783.99, d: 0.12 }, // G5
            { f: 659.25, d: 0.12 }, // E5
            { f: 1046.50, d: 0.08 },// C6
            { f: 783.99, d: 0.16 }, // G5
            { f: 659.25, d: 0.20 }  // E5
        ];

        let offset = this.ctx.currentTime + 0.05;
        notes.forEach(note => {
            const osc = this.ctx.createOscillator();
            const gain = this.ctx.createGain();

            osc.type = 'square';
            osc.frequency.setValueAtTime(note.f, offset);

            gain.gain.setValueAtTime(0.18, offset);
            gain.gain.exponentialRampToValueAtTime(0.01, offset + note.d);

            osc.connect(gain);
            gain.connect(this.ctx.destination);

            osc.start(offset);
            osc.stop(offset + note.d);

            offset += note.d + 0.02;
        });
    }

    playFruit() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;

        const now = this.ctx.currentTime;
        const notes = [523.25, 659.25, 783.99, 1046.50];
        notes.forEach((f, i) => {
            const osc = this.ctx.createOscillator();
            const gain = this.ctx.createGain();
            const t = now + (i * 0.05);

            osc.type = 'triangle';
            osc.frequency.setValueAtTime(f, t);

            gain.gain.setValueAtTime(0.2, t);
            gain.gain.linearRampToValueAtTime(0.01, t + 0.05);

            osc.connect(gain);
            gain.connect(this.ctx.destination);

            osc.start(t);
            osc.stop(t + 0.05);
        });
    }
}
