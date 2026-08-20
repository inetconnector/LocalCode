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
