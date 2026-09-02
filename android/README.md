# LocalCode Remote for Android

This directory contains the native Android shell for LocalCode Remote.

## Security model

- Discovery uses DNS-SD/mDNS service type `_localcode._tcp.`.
- Non-loopback Remote endpoints are HTTPS-only.
- LocalCode advertises the SHA-256 fingerprint of its locally generated TLS certificate in the DNS-SD TXT record.
- The Android WebView accepts the self-signed LocalCode certificate only when its SHA-256 fingerprint matches the value discovered over DNS-SD or supplied in a `localcode://pair` deep link.
- Cleartext WebView traffic is disabled in the Android manifest.
- The existing eight-digit LocalCode pairing code is still entered in the Remote web UI after the transport endpoint has been discovered.
- WebView file inputs are handed to Android's native file chooser through `onShowFileChooser`; selected URIs stay inside Android's normal picker grants and are passed back to the Remote web app as browser file attachments.
- Voice input uses Android's `RecognizerIntent` through a narrow `LocalCodeAndroid` JavaScript bridge. The bridge only starts speech recognition and returns recognized text to the prompt field.
- The bridge checks that a speech-recognition activity is available before launching it. File-picker and speech-launch failures are shown in the visible Remote WebView rather than in the hidden discovery panel.
- Pending WebView file-chooser callbacks are cancelled before a replacement chooser starts and during Activity teardown so the attachment control cannot remain stuck behind an orphaned callback.
- Native discovery/status controls follow the Android display language: German on a German device, English otherwise. The Remote web app keeps its own matching German/English catalog.
- The Android shell stores the last accepted Remote URL and TLS fingerprint in private app preferences. On the next app launch it opens that connection directly; if the main WebView load fails, the discovery panel appears and discovery starts again.
- The automatic discovery button starts mDNS and a bounded parallel direct private-LAN fallback scan on the Remote port. The fallback checks `/remote/api/discovery` first so a TLS fingerprint can be recovered, then `/remote/api/ping` for older reachable instances.

## Mobile composer contract

The Remote composer keeps all editing on the Windows LocalCode instance while the phone supplies input. The attachment button opens the native Android picker, multiple selected files are passed through the Remote attachment validation, and choosing the same file again is supported because the browser file input is reset after each selection. The microphone appends recognized text to the current prompt without sending it automatically. The send path is guarded against duplicate submissions, forwards the current project/thread/model and attachments together, clears the prompt and attachments only after the chat request succeeds, and leaves them intact when the request fails.

The Remote UI starts on **New task** and keeps that tab first in the native Android/WebView navigation order. Approval requests appear as an in-place popup over the current view instead of consuming a permanent navigation tab. Transient run events such as model-step progress and currently-running tool output stay visible while the Windows agent is running, then disappear from the rendered mobile history after completion. The UI also wires pairing, navigation tabs, project/task selection, project-management actions, approval buttons, engine selection, stop, attachment removal, voice input and send controls through explicit handlers. Go regression tests keep these central mobile-control bindings and Android bridge invariants from silently disappearing.

## QR / deep-link handoff

The Android app registers this URI shape:

```text
localcode://pair?url=https%3A%2F%2F192.168.1.10%3A32146%2Fremote&fp=<SHA256_CERT_FINGERPRINT>
```

A QR scanner or the Android system camera can hand such a URI directly to LocalCode Remote. The app rejects non-HTTPS URLs and requires a fingerprint for deep-link launches.

## Network permissions

The app targets Android API 36. It declares `NEARBY_WIFI_DEVICES` for Android 13+ Local Network Protection compatibility and acquires a Wi-Fi multicast lock while discovery is active for older devices that require it.

## Reproducible CI build

`scripts/build-android.ps1` uses only the Android SDK/JDK already installed on the GitHub Windows runner:

1. `aapt2` links the manifest against Android API 36.
2. `javac` compiles the Java source against `android.jar`.
3. `d8` creates `classes.dex`.
4. `aapt` inserts the DEX into the APK.
5. `zipalign` aligns it.
6. `apksigner` signs and verifies a throwaway debug APK.

No Gradle wrapper, Maven dependency, downloaded Android library, or committed signing key is required.
