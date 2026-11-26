# LocalCode Zero-Knowledge E2EE Relay — Self-Hosting Guide (DE / EN)

## Deutsch
`localcode-relay` ist ein minimaler, vollständig offener **Zero-Knowledge Ende-zu-Ende-Relay-Server** (X25519 + HKDF-SHA256 + AES-256-GCM). Der Relay leitet ausschließlich verschlüsselte Chiffretexte weiter und sieht **keinen Code, keine Prompts, keine Diffs und keine Auth-Tokens**.

### Schnellstart mit Docker + automatischem HTTPS (Caddy)
```bash
cd deploy/relay
RELAY_DOMAIN=relay.example.com docker compose up -d --build
```

### Kopplung von Workstation & Handy
1. In LocalCode `POST /api/relay/pair` (oder in NomaMica Studio `Alt+6` → **E2EE-Pairing erzeugen**) mit `relay_url: "https://relay.example.com"` auslösen.
2. Den erzeugten `localcode://relay-pair?...` Deep-Link/QR-Code mit der Android-App öffnen (das Auth-Secret wird im Android Keystore per AES-256-GCM gespeichert).

---

## English
`localcode-relay` is a minimal, open-source **Zero-Knowledge End-to-End Encrypted Relay** (X25519 + HKDF-SHA256 + AES-256-GCM). It forwards ciphertext envelopes only and never sees code, prompts, diffs, or authentication tokens.

### Quickstart with Docker + automatic HTTPS (Caddy)
```bash
cd deploy/relay
RELAY_DOMAIN=relay.example.com docker compose up -d --build
```

### Pairing Workstation & Mobile Companion
1. Trigger `POST /api/relay/pair` in LocalCode (or in NomaMica Studio `Alt+6` → **Generate E2EE Pairing**) with `relay_url: "https://relay.example.com"`.
2. Open the resulting `localcode://relay-pair?...` deep link / QR code in the Android app (auth secret is stored AES-256-GCM encrypted in Android Keystore).
