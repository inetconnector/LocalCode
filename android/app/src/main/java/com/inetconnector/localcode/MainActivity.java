// SPDX-License-Identifier: Apache-2.0
package com.inetconnector.localcode;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.net.http.SslCertificate;
import android.net.http.SslError;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.Bundle;
import android.os.VibrationEffect;
import android.os.Vibrator;
import android.provider.MediaStore;
import android.speech.RecognizerIntent;
import android.speech.tts.TextToSpeech;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.JavascriptInterface;
import android.webkit.JsPromptResult;
import android.webkit.JsResult;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.InetAddress;
import java.net.Inet4Address;
import java.net.InterfaceAddress;
import java.net.NetworkInterface;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.security.cert.X509Certificate;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashSet;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import javax.net.ssl.HostnameVerifier;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

import org.json.JSONArray;
import org.json.JSONObject;

@SuppressWarnings("deprecation")
public final class MainActivity extends Activity {
    private static final String SERVICE_TYPE = "_localcode._tcp.";
    private static final String PREFS_NAME = "localcode_remote";
    private static final String PREF_REMOTE_URL = "remote_url";
    private static final String PREF_TLS_FINGERPRINT = "tls_fingerprint";
    private static final int DEFAULT_REMOTE_PORT = 32146;
    private static final int REQUEST_NEARBY = 701;
    private static final int REQUEST_FILE_CHOOSER = 702;
    private static final int REQUEST_SPEECH = 703;
    private static final int REQUEST_QR_SCAN = 704;

    private NsdManager nsdManager;
    private NsdManager.DiscoveryListener discoveryListener;
    private WifiManager.MulticastLock multicastLock;
    private WebView webView;
    private LinearLayout discoveryPanel;
    private TextView status;
    private EditText manualUrl;
    private EditText manualFingerprint;
    private ValueCallback<Uri[]> filePathCallback;
    private SharedPreferences preferences;
    private TextToSpeech tts;
    private String expectedFingerprint = "";
    private String currentRemoteUrl = "";
    private boolean discovering;
    private volatile boolean scanningLan;


    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            getWindow().setStatusBarColor(0xFF0D0D0D);
            getWindow().setNavigationBarColor(0xFF0D0D0D);
        }
        nsdManager = (NsdManager) getSystemService(Context.NSD_SERVICE);
        preferences = getSharedPreferences(PREFS_NAME, MODE_PRIVATE);
        tts = new TextToSpeech(this, status -> {
            if (status == TextToSpeech.SUCCESS && tts != null) {
                tts.setLanguage(Locale.getDefault());
            }
        });
        buildUi();

        boolean handled = handleIntent(getIntent());
        if (handled) return;
        loadSavedConnection();
        if (!currentRemoteUrl.isEmpty()) {
            openRemote(currentRemoteUrl);
        } else {
            requestDiscoveryPermissionAndStart();
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handleIntent(intent);
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setLayoutParams(new ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        discoveryPanel = new LinearLayout(this);
        discoveryPanel.setOrientation(LinearLayout.VERTICAL);
        discoveryPanel.setGravity(Gravity.CENTER_HORIZONTAL);
        int pad = dp(20);
        discoveryPanel.setPadding(pad, pad, pad, pad);

        TextView title = new TextView(this);
        title.setText("LocalCode Remote");
        title.setTextSize(24f);
        discoveryPanel.addView(title, fullWidthWrap());

        status = new TextView(this);
        status.setText(tr("Suche LocalCode im lokalen Netzwerk …", "Searching for LocalCode on the local network …"));
        status.setPadding(0, dp(10), 0, dp(12));
        discoveryPanel.addView(status, fullWidthWrap());

        Button scanQr = new Button(this);
        scanQr.setText(tr("📷 QR-Code scannen", "📷 Scan QR code"));
        scanQr.setOnClickListener(v -> launchQrScanner());
        discoveryPanel.addView(scanQr, fullWidthWrap());

        Button discover = new Button(this);
        discover.setText(tr("LocalCode automatisch suchen", "Find LocalCode automatically"));
        discover.setOnClickListener(v -> requestDiscoveryPermissionAndStart());
        discoveryPanel.addView(discover, fullWidthWrap());

        manualUrl = new EditText(this);
        manualUrl.setSingleLine(true);
        manualUrl.setHint("http://192.168.1.94:32146/remote");
        discoveryPanel.addView(manualUrl, fullWidthWrap());

        manualFingerprint = new EditText(this);
        manualFingerprint.setSingleLine(true);
        manualFingerprint.setHint(tr("TLS-Fingerprint (optional bei HTTP)", "TLS fingerprint (optional for HTTP)"));
        discoveryPanel.addView(manualFingerprint, fullWidthWrap());

        Button open = new Button(this);
        open.setText(tr("Adresse öffnen", "Open address"));
        open.setOnClickListener(v -> {
            String value = manualUrl.getText().toString().trim();
            String fp = normalizeFingerprint(manualFingerprint.getText().toString());
            if (isAllowedRemoteUrl(value) && validFingerprint(fp)) {
                expectedFingerprint = fp;
                openRemote(value);
            } else {
                setStatus(tr(
                        "Manuell sind nur private HTTPS-IP-Adressen mit gültigem SHA-256-Fingerprint erlaubt.",
                        "Manual setup allows private HTTPS IP addresses with a valid SHA-256 fingerprint only."));
            }
        });
        discoveryPanel.addView(open, fullWidthWrap());

        TextView qrHint = new TextView(this);
        qrHint.setText(tr(
                "Am einfachsten: QR-/Pair-Link verwenden. URL und TLS-Fingerprint werden dann automatisch und zusammen übernommen.",
                "Easiest: use the QR/pair link. The URL and TLS fingerprint are then transferred together automatically."));
        qrHint.setPadding(0, dp(12), 0, 0);
        discoveryPanel.addView(qrHint, fullWidthWrap());

        root.addView(discoveryPanel, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        webView = new WebView(this);
        configureWebView();
        webView.setVisibility(View.GONE);
        root.addView(webView, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f));
        setContentView(root);
    }

    private LinearLayout.LayoutParams fullWidthWrap() {
        return new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
    }

    private void configureWebView() {
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSafeBrowsingEnabled(true);
        webView.addJavascriptInterface(new AndroidBridge(), "LocalCodeAndroid");
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                cancelPendingFileChooser();
                filePathCallback = callback;
                Intent intent;
                try {
                    intent = params.createIntent();
                } catch (RuntimeException ex) {
                    cancelPendingFileChooser();
                    showRemoteError(
                            "Dateiauswahl konnte nicht geöffnet werden.",
                            "The file picker could not be opened.",
                            ex);
                    return true;
                }
                try {
                    startActivityForResult(intent, REQUEST_FILE_CHOOSER);
                    return true;
                } catch (RuntimeException ex) {
                    cancelPendingFileChooser();
                    showRemoteError(
                            "Keine passende Dateiauswahl-App gefunden.",
                            "No compatible file picker app was found.",
                            ex);
                    return true;
                }
            }

            @Override
            public boolean onJsAlert(WebView view, String url, String message, JsResult result) {
                new AlertDialog.Builder(MainActivity.this)
                        .setTitle("LocalCode")
                        .setMessage(message)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                return true;
            }

            @Override
            public boolean onJsConfirm(WebView view, String url, String message, JsResult result) {
                new AlertDialog.Builder(MainActivity.this)
                        .setTitle("LocalCode")
                        .setMessage(message)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm())
                        .setNegativeButton(android.R.string.cancel, (dialog, which) -> result.cancel())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                return true;
            }

            @Override
            public boolean onJsPrompt(WebView view, String url, String message, String defaultValue, JsPromptResult result) {
                final EditText input = new EditText(MainActivity.this);
                input.setText(defaultValue != null ? defaultValue : "");
                new AlertDialog.Builder(MainActivity.this)
                        .setTitle("LocalCode")
                        .setMessage(message)
                        .setView(input)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm(input.getText().toString()))
                        .setNegativeButton(android.R.string.cancel, (dialog, which) -> result.cancel())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                return true;
            }
        });
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return !sameRemoteOrigin(request.getUrl());
            }

            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                String observed = fingerprint(error.getCertificate());
                if ((validFingerprint(expectedFingerprint) && expectedFingerprint.equalsIgnoreCase(observed)) || isPrivateHost(currentRemoteUrl)) {
                    expectedFingerprint = observed;
                    persistConnection(currentRemoteUrl, expectedFingerprint);
                    handler.proceed();
                } else {
                    handler.cancel();
                    runOnUiThread(() -> {
                        webView.setVisibility(View.GONE);
                        discoveryPanel.setVisibility(View.VISIBLE);
                        setStatus(tr(
                                "TLS-Zertifikat nicht bestätigt. Erwartet: " + printable(expectedFingerprint) + " · Empfangen: " + printable(observed),
                                "TLS certificate not confirmed. Expected: " + printable(expectedFingerprint) + " · Received: " + printable(observed)));
                    });
                }
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request != null && request.isForMainFrame()) {
                    runOnUiThread(() -> {
                        webView.setVisibility(View.GONE);
                        discoveryPanel.setVisibility(View.VISIBLE);
                        setStatus(tr(
                                "Gespeicherte Verbindung nicht erreichbar. Suche LocalCode erneut …",
                                "Saved connection is not reachable. Searching for LocalCode again …"));
                        requestDiscoveryPermissionAndStart();
                    });
                }
            }
        });
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == REQUEST_FILE_CHOOSER) {
            ValueCallback<Uri[]> callback = filePathCallback;
            filePathCallback = null;
            if (callback != null) {
                try {
                    Uri[] results = resultCode == RESULT_OK ? WebChromeClient.FileChooserParams.parseResult(resultCode, data) : null;
                    callback.onReceiveValue(results);
                } catch (RuntimeException ex) {


                    callback.onReceiveValue(null);
                    showRemoteError(
                            "Die ausgewählten Dateien konnten nicht übernommen werden.",
                            "The selected files could not be attached.",
                            ex);
                }
            }
            return;
        }
        if (requestCode == REQUEST_SPEECH) {
            if (resultCode == RESULT_OK && data != null) {
                ArrayList<String> matches = data.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS);
                if (matches != null && !matches.isEmpty() && matches.get(0) != null && !matches.get(0).trim().isEmpty()) {
                    deliverVoiceText(matches.get(0));
                } else {
                    showRemoteError(
                            "Die Spracheingabe lieferte keinen Text.",
                            "Voice input returned no text.",
                            null);
                }
            }
            return;
        }
        if (requestCode == REQUEST_QR_SCAN) {
            if (resultCode == RESULT_OK && data != null) {
                String contents = data.getStringExtra("SCAN_RESULT");
                if (contents != null && !contents.trim().isEmpty()) {
                    handleScannedContent(contents.trim());
                }
            }
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    private final class AndroidBridge {
        @JavascriptInterface
        public void startVoiceInput() {
            runOnUiThread(() -> startVoiceRecognizer());
        }

        @JavascriptInterface
        public String runDiagnostics() {
            JSONObject diag = new JSONObject();
            try {
                diag.put("device_model", Build.MODEL);
                diag.put("android_version", Build.VERSION.RELEASE);
                diag.put("sdk_int", Build.VERSION.SDK_INT);
                diag.put("voice_available", new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).resolveActivity(getPackageManager()) != null);
                diag.put("discovering", discovering);
                diag.put("current_remote_url", currentRemoteUrl);
                diag.put("ok", true);
            } catch (Exception e) {
                try { diag.put("error", e.getMessage()); } catch (Exception ignored) {}
            }
            return diag.toString();
        }

        @JavascriptInterface
        public void sendVoiceTest(String sampleText) {
            runOnUiThread(() -> deliverVoiceText(sampleText));
        }

        @JavascriptInterface
        public void startQrScan() {
            runOnUiThread(() -> launchQrScanner());
        }

        @JavascriptInterface
        public String getBridgeVersion() {
            return "2.1";
        }

        @JavascriptInterface
        public void vibrate(int milliseconds) {
            runOnUiThread(() -> {
                try {
                    Vibrator v = (Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
                    if (v != null && v.hasVibrator()) {
                        int dur = Math.max(1, Math.min(milliseconds, 500));
                        if (Build.VERSION.SDK_INT >= 26) {
                            v.vibrate(VibrationEffect.createOneShot(dur, VibrationEffect.DEFAULT_AMPLITUDE));
                        } else {
                            v.vibrate(dur);
                        }
                    }
                } catch (Exception ignored) {}
            });
        }

        @JavascriptInterface
        public void speak(String text) {
            runOnUiThread(() -> startSpeaking(text));
        }

        @JavascriptInterface
        public void stopSpeaking() {
            runOnUiThread(() -> stopTts());
        }

        @JavascriptInterface
        public boolean isTtsAvailable() {
            return tts != null;
        }

        @JavascriptInterface
        public void resetConnection() {
            runOnUiThread(() -> {
                currentRemoteUrl = "";
                expectedFingerprint = "";
                clearSavedConnection();
                if (webView != null) webView.setVisibility(View.GONE);
                if (discoveryPanel != null) discoveryPanel.setVisibility(View.VISIBLE);
            });
        }
    }

    private void startSpeaking(String text) {
        if (tts == null || text == null || text.trim().isEmpty()) return;
        try {
            String cleanText = text.trim();
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
                tts.speak(cleanText, TextToSpeech.QUEUE_FLUSH, null, "LocalCodeTTS");
            } else {
                tts.speak(cleanText, TextToSpeech.QUEUE_FLUSH, null);
            }
        } catch (Exception ignored) {}
    }

    private void stopTts() {
        if (tts != null) {
            try { tts.stop(); } catch (Exception ignored) {}
        }
    }


    private void startVoiceRecognizer() {
        Intent intent = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
        intent.putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, false);
        intent.putExtra(RecognizerIntent.EXTRA_PROMPT, "LocalCode");
        if (intent.resolveActivity(getPackageManager()) == null) {
            showRemoteError(
                    "Keine Spracheingabe-App gefunden.",
                    "No voice input app was found.",
                    null);
            return;
        }
        try {
            startActivityForResult(intent, REQUEST_SPEECH);
        } catch (RuntimeException ex) {
            showRemoteError(
                    "Spracheingabe konnte nicht gestartet werden.",
                    "Voice input could not be started.",
                    ex);
        }
    }

    private void deliverVoiceText(String text) {
        if (webView == null) return;
        String script = "window.localCodeVoiceResult&&window.localCodeVoiceResult(" + JSONObject.quote(text == null ? "" : text) + ")";
        webView.evaluateJavascript(script, null);
    }

    private void showRemoteError(String german, String english, RuntimeException error) {
        String message = tr(german, english);
        String detail = error == null || error.getMessage() == null ? "" : error.getMessage().trim();
        if (!detail.isEmpty()) message += " " + detail;
        final String visibleMessage = message;
        runOnUiThread(() -> {
            if (webView != null && webView.getVisibility() == View.VISIBLE) {
                webView.evaluateJavascript("window.alert(" + JSONObject.quote(visibleMessage) + ")", null);
            } else if (status != null) {
                status.setText(visibleMessage);
            }
        });
    }

    private void launchQrScanner() {
        Intent scanIntent = new Intent("com.google.zxing.client.android.SCAN");
        scanIntent.putExtra("SCAN_MODE", "QR_CODE_MODE");
        if (scanIntent.resolveActivity(getPackageManager()) != null) {
            try {
                startActivityForResult(scanIntent, REQUEST_QR_SCAN);
                return;
            } catch (Exception ignored) {}
        }
        try {
            Intent cameraIntent = new Intent(MediaStore.ACTION_IMAGE_CAPTURE);
            startActivity(cameraIntent);
            setStatus(tr(
                    "Kamera geöffnet: QR-Code auf dem PC scannen oder Link antippen.",
                    "Camera opened: scan the QR code on your PC or tap the link."));
        } catch (Exception ex) {
            setStatus(tr(
                    "Bitte den QR-Code mit der Smartphone-Kamera scannen.",
                    "Please scan the QR code using your phone camera."));
        }
    }

    private void handleScannedContent(String raw) {
        if (raw == null || raw.trim().isEmpty()) return;
        Uri uri = Uri.parse(raw.trim());
        if ("localcode".equalsIgnoreCase(uri.getScheme())) {
            Intent intent = new Intent(Intent.ACTION_VIEW, uri);
            handleIntent(intent);
        } else if (isAllowedRemoteUrl(raw.trim())) {
            openRemote(raw.trim());
        }
    }

    private void cancelPendingFileChooser() {
        ValueCallback<Uri[]> callback = filePathCallback;
        filePathCallback = null;
        if (callback != null) callback.onReceiveValue(null);
    }

    private boolean sameRemoteOrigin(Uri candidate) {
        if (candidate == null || currentRemoteUrl.isEmpty()) return false;
        Uri expected = Uri.parse(currentRemoteUrl);
        if (!"https".equalsIgnoreCase(candidate.getScheme()) || !"https".equalsIgnoreCase(expected.getScheme())) return false;
        String candidateHost = candidate.getHost();
        String expectedHost = expected.getHost();
        if (candidateHost == null || expectedHost == null || !candidateHost.equalsIgnoreCase(expectedHost)) return false;
        return effectiveHttpsPort(candidate) == effectiveHttpsPort(expected);
    }

    private static int effectiveHttpsPort(Uri uri) {
        return uri.getPort() > 0 ? uri.getPort() : 443;
    }

    private void requestDiscoveryPermissionAndStart() {
        startLanProbeDiscovery();
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.NEARBY_WIFI_DEVICES) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.NEARBY_WIFI_DEVICES}, REQUEST_NEARBY);
            return;
        }
        startDiscovery();
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == REQUEST_NEARBY) {
            if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                startDiscovery();
            } else {
                setStatus(tr(
                        "mDNS nicht freigegeben. Prüfe bekannte LAN-Adressen direkt …",
                        "mDNS was not allowed. Checking known LAN addresses directly …"));
            }
        }
    }

    private void startDiscovery() {
        if (nsdManager == null || discovering) {
            return;
        }
        acquireMulticastLock();
        setStatus(tr("Suche LocalCode per mDNS …", "Searching for LocalCode via mDNS …"));
        discoveryListener = new NsdManager.DiscoveryListener() {
            @Override public void onDiscoveryStarted(String serviceType) { discovering = true; }
            @Override public void onDiscoveryStopped(String serviceType) { discovering = false; releaseMulticastLock(); }
            @Override public void onStartDiscoveryFailed(String serviceType, int errorCode) {
                discovering = false;
                releaseMulticastLock();
                setStatus(tr(
                        "mDNS-Suche konnte nicht gestartet werden (" + errorCode + ").",
                        "mDNS discovery could not be started (" + errorCode + ")."));
            }
            @Override public void onStopDiscoveryFailed(String serviceType, int errorCode) { discovering = false; releaseMulticastLock(); }
            @Override public void onServiceLost(NsdServiceInfo serviceInfo) { }
            @Override public void onServiceFound(NsdServiceInfo serviceInfo) {
                if (!serviceInfo.getServiceType().toLowerCase(Locale.ROOT).contains("_localcode._tcp")) {
                    return;
                }
                resolve(serviceInfo);
            }
        };
        try {
            nsdManager.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, discoveryListener);
        } catch (RuntimeException ex) {
            discovering = false;
            releaseMulticastLock();
            setStatus(tr("mDNS-Suche fehlgeschlagen: ", "mDNS discovery failed: ") + safeMessage(ex));
        }
    }

    private void startLanProbeDiscovery() {
        if (scanningLan) return;
        scanningLan = true;
        int port = discoveryPort();
        setStatus(tr("Suche LocalCode im lokalen Netzwerk …", "Searching for LocalCode on the local network …"));
        new Thread(() -> {
            AtomicBoolean found = new AtomicBoolean(false);
            ExecutorService pool = Executors.newFixedThreadPool(24);
            try {
                for (String host : lanProbeCandidates()) {
                    pool.execute(() -> {
                        if (found.get()) return;
                        ProbeResult result = probeLocalCode(host, port);
                        if (result != null && found.compareAndSet(false, true)) {
                            runOnUiThread(() -> {
                                expectedFingerprint = result.fingerprint;
                                openRemote(result.url);
                            });
                        }
                    });
                }
                pool.shutdown();
                if (!pool.awaitTermination(8, TimeUnit.SECONDS)) pool.shutdownNow();
                if (!found.get()) {
                    setStatus(tr(
                            "Keine LocalCode-Instanz gefunden. QR-Code scannen oder Adresse vom PC eingeben.",
                            "No LocalCode instance found. Scan the QR code or enter the PC address."));
                }
            } catch (InterruptedException ex) {
                Thread.currentThread().interrupt();
            } finally {
                pool.shutdownNow();
                scanningLan = false;
            }
        }, "LocalCodeLanProbe").start();
    }

    private int discoveryPort() {
        int port = portFromText(manualUrl == null ? "" : manualUrl.getText().toString());
        if (port <= 0) port = portFromText(currentRemoteUrl);
        return port > 0 ? port : DEFAULT_REMOTE_PORT;
    }

    private static int portFromText(String value) {
        if (value == null || value.trim().isEmpty()) return 0;
        try {
            Uri uri = Uri.parse(value.trim());
            return uri.getPort() > 0 ? uri.getPort() : 0;
        } catch (Exception ex) {
            return 0;
        }
    }

    private ArrayList<String> lanProbeCandidates() {
        ArrayList<String> out = new ArrayList<>();
        Set<String> seen = new HashSet<>();
        addCandidateHost(out, seen, hostFromText(manualUrl == null ? "" : manualUrl.getText().toString()));
        addCandidateHost(out, seen, hostFromText(currentRemoteUrl));
        try {
            for (NetworkInterface iface : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                if (!iface.isUp() || iface.isLoopback()) continue;
                for (InterfaceAddress address : iface.getInterfaceAddresses()) {
                    if (!(address.getAddress() instanceof Inet4Address)) continue;
                    int prefix = address.getNetworkPrefixLength();
                    if (prefix < 24 || prefix > 30) prefix = 24;
                    addSubnetCandidates(out, seen, (Inet4Address) address.getAddress(), prefix);
                    if (out.size() >= 512) return out;
                }
            }
        } catch (Exception ignored) {
        }
        return out;
    }

    private static String hostFromText(String value) {
        if (value == null || value.trim().isEmpty()) return "";
        try {
            return Uri.parse(value.trim()).getHost();
        } catch (Exception ex) {
            return "";
        }
    }

    private static void addSubnetCandidates(ArrayList<String> out, Set<String> seen, Inet4Address address, int prefix) {
        byte[] raw = address.getAddress();
        int ip = ((raw[0] & 0xff) << 24) | ((raw[1] & 0xff) << 16) | ((raw[2] & 0xff) << 8) | (raw[3] & 0xff);
        int mask = prefix == 0 ? 0 : (int) (0xffffffffL << (32 - prefix));
        int network = ip & mask;
        int broadcast = network | ~mask;
        for (int value = network + 1; value < broadcast && out.size() < 512; value++) {
            if (value == ip) continue;
            String host = ((value >>> 24) & 0xff) + "." + ((value >>> 16) & 0xff) + "." + ((value >>> 8) & 0xff) + "." + (value & 0xff);
            addCandidateHost(out, seen, host);
        }
    }

    private static void addCandidateHost(ArrayList<String> out, Set<String> seen, String host) {
        if (host == null || host.trim().isEmpty()) return;
        host = host.trim();
        if (!isLiteralIPv4(host)) return;
        try {
            if (!isPrivateAddress(InetAddress.getByName(host))) return;
        } catch (Exception ex) {
            return;
        }
        if (seen.add(host)) out.add(host);
    }

    private ProbeResult probeLocalCode(String host, int port) {
        ProbeResult result = probeLocalCodeEndpoint("https", host, port, true);
        if (result != null) return result;
        return probeLocalCodeEndpoint("http", host, port, false);
    }

    private ProbeResult probeLocalCodeEndpoint(String scheme, String host, int port, boolean trustDiscoveryCertificate) {
        for (String path : new String[]{"/remote/api/discovery", "/remote/api/ping"}) {
            HttpURLConnection connection = null;
            try {
                URL url = new URL(scheme + "://" + host + ":" + port + path);
                connection = (HttpURLConnection) url.openConnection();
                connection.setConnectTimeout(350);
                connection.setReadTimeout(550);
                connection.setRequestProperty("Accept", "application/json");
                if (connection instanceof HttpsURLConnection && trustDiscoveryCertificate) {
                    HttpsURLConnection https = (HttpsURLConnection) connection;
                    https.setSSLSocketFactory(discoverySSLContext().getSocketFactory());
                    https.setHostnameVerifier(discoveryHostnameVerifier());
                }
                int code = connection.getResponseCode();
                if (code != 200) continue;
                String body = readSmallResponse(connection.getInputStream());
                JSONObject json = new JSONObject(body);
                if (!json.optString("app", "").contains("LocalCode Remote")) continue;
                String fingerprint = normalizeFingerprint(json.optString("tls_fingerprint", ""));
                String target = discoveryURLFromJSON(json, scheme + "://" + host + ":" + port + "/remote");
                if (target == null || !isAllowedRemoteUrl(target)) continue;
                if ("https".equalsIgnoreCase(scheme) && path.endsWith("/discovery") && !validFingerprint(fingerprint)) continue;
                return new ProbeResult(target, fingerprint);
            } catch (Exception ignored) {
            } finally {
                if (connection != null) connection.disconnect();
            }
        }
        return null;
    }

    private static String discoveryURLFromJSON(JSONObject json, String fallback) {
        JSONArray urls = json.optJSONArray("remote_urls");
        if (urls != null) {
            for (int i = 0; i < urls.length(); i++) {
                String value = urls.optString(i, "");
                if (isAllowedRemoteUrl(value)) return value;
            }
        }
        return fallback;
    }

    private static String readSmallResponse(InputStream input) throws Exception {
        try (InputStream in = input; ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            byte[] buf = new byte[1024];
            int total = 0;
            int n;
            while ((n = in.read(buf)) >= 0 && total < 8192) {
                out.write(buf, 0, n);
                total += n;
            }
            return out.toString("UTF-8");
        }
    }

    private static SSLContext discoverySSLContext() throws Exception {
        TrustManager[] trustManagers = new TrustManager[]{new X509TrustManager() {
            @Override public void checkClientTrusted(X509Certificate[] chain, String authType) { }
            @Override public void checkServerTrusted(X509Certificate[] chain, String authType) { }
            @Override public X509Certificate[] getAcceptedIssuers() { return new X509Certificate[0]; }
        }};
        SSLContext context = SSLContext.getInstance("TLS");
        context.init(null, trustManagers, new SecureRandom());
        return context;
    }

    private static HostnameVerifier discoveryHostnameVerifier() {
        return (hostname, session) -> true;
    }

    private static final class ProbeResult {
        final String url;
        final String fingerprint;

        ProbeResult(String url, String fingerprint) {
            this.url = url;
            this.fingerprint = fingerprint;
        }
    }

    private void resolve(NsdServiceInfo serviceInfo) {
        try {
            nsdManager.resolveService(serviceInfo, new NsdManager.ResolveListener() {
                @Override public void onResolveFailed(NsdServiceInfo info, int errorCode) {
                    setStatus(tr(
                            "LocalCode gefunden, Auflösung fehlgeschlagen (" + errorCode + ").",
                            "LocalCode was found, but resolving it failed (" + errorCode + ")."));
                }
                @Override public void onServiceResolved(NsdServiceInfo info) {
                    InetAddress host = info.getHost();
                    if (host == null || info.getPort() <= 0 || !isPrivateAddress(host)) {
                        setStatus(tr(
                                "LocalCode-Dienst enthält keine verwendbare private LAN-Adresse.",
                                "The LocalCode service does not contain a usable private LAN address."));
                        return;
                    }
                    Map<String, byte[]> attrs = info.getAttributes();
                    String tls = attribute(attrs, "tls");
                    String fp = normalizeFingerprint(attribute(attrs, "fp"));
                    String path = attribute(attrs, "path");
                    if (!"1".equals(tls) || !validFingerprint(fp)) {
                        setStatus(tr(
                                "Unsicherer LocalCode-Dienst verworfen: gültiger TLS-Fingerprint fehlt.",
                                "Unsafe LocalCode service rejected: a valid TLS fingerprint is missing."));
                        return;
                    }
                    if (path.isEmpty()) path = "/remote";
                    String address = host.getHostAddress();
                    if (address.contains(":")) address = "[" + address + "]";
                    expectedFingerprint = fp;
                    String target = "https://" + address + ":" + info.getPort() + path;
                    stopDiscovery();
                    openRemote(target);
                }
            });
        } catch (RuntimeException ex) {
            setStatus(tr("LocalCode-Auflösung fehlgeschlagen: ", "Resolving LocalCode failed: ") + safeMessage(ex));
        }
    }

    private boolean handleIntent(Intent intent) {
        if (intent == null) return false;
        String directUrl = intent.getStringExtra("connect_url");
        if (directUrl != null && !directUrl.trim().isEmpty() && isAllowedRemoteUrl(directUrl.trim())) {
            openRemote(directUrl.trim());
            return true;
        }
        if (intent.getData() == null) return false;
        Uri data = intent.getData();
        if (!"localcode".equalsIgnoreCase(data.getScheme()) || !"pair".equalsIgnoreCase(data.getHost())) return false;
        String target = data.getQueryParameter("url");
        String fp = normalizeFingerprint(data.getQueryParameter("fp"));
        String code = data.getQueryParameter("code");
        if (target != null && isAllowedRemoteUrl(target)) {
            expectedFingerprint = fp;
            if (code != null && !code.trim().isEmpty()) {
                String separator = target.contains("#") ? "&" : "#";
                openRemote(target + separator + "code=" + Uri.encode(code.trim()));
            } else {
                openRemote(target);
            }
        } else {
            setStatus(tr(
                    "Der QR-/Deep-Link ist unvollständig oder unsicher.",
                    "The QR/deep link is incomplete or unsafe."));
        }
        return true;
    }

    private void openRemote(String target) {
        if (!isAllowedRemoteUrl(target)) {
            setStatus(tr(
                    "Unsichere Remote-Adresse verworfen.",
                    "Unsafe Remote address rejected."));
            return;
        }
        currentRemoteUrl = target;
        persistConnection(target, expectedFingerprint);
        runOnUiThread(() -> {
            discoveryPanel.setVisibility(View.GONE);
            webView.setVisibility(View.VISIBLE);
            webView.loadUrl(target);
        });
    }

    private void loadSavedConnection() {
        if (preferences == null) return;
        currentRemoteUrl = preferences.getString(PREF_REMOTE_URL, "");
        expectedFingerprint = normalizeFingerprint(preferences.getString(PREF_TLS_FINGERPRINT, ""));
        if (!isAllowedRemoteUrl(currentRemoteUrl)) {
            currentRemoteUrl = "";
            expectedFingerprint = "";
        }
    }

    private void persistConnection(String target, String fingerprint) {
        if (preferences == null || !isAllowedRemoteUrl(target)) return;
        preferences.edit()
                .putString(PREF_REMOTE_URL, target)
                .putString(PREF_TLS_FINGERPRINT, normalizeFingerprint(fingerprint))
                .apply();
    }

    private void clearSavedConnection() {
        if (preferences == null) return;
        preferences.edit().remove(PREF_REMOTE_URL).remove(PREF_TLS_FINGERPRINT).apply();
    }

    private static boolean isAllowedRemoteUrl(String value) {
        if (value == null || value.trim().isEmpty()) return false;
        Uri uri = Uri.parse(value.trim());
        String scheme = uri.getScheme();
        if ((!"https".equalsIgnoreCase(scheme) && !"http".equalsIgnoreCase(scheme)) || uri.getHost() == null || uri.getUserInfo() != null) return false;
        String host = uri.getHost();
        if ("localhost".equalsIgnoreCase(host) || "127.0.0.1".equals(host)) return true;
        if (!isLiteralIPv4(host) && !host.contains(":")) return false;
        try {
            return isPrivateAddress(InetAddress.getByName(host));
        } catch (Exception ex) {
            return false;
        }
    }

    private static boolean isLiteralIPv4(String host) {
        if (host == null) return false;
        String[] parts = host.split("\\.", -1);
        if (parts.length != 4) return false;
        for (String part : parts) {
            if (part.isEmpty() || part.length() > 3) return false;
            for (int i = 0; i < part.length(); i++) {
                if (!Character.isDigit(part.charAt(i))) return false;
            }
            try {
                int value = Integer.parseInt(part);
                if (value < 0 || value > 255) return false;
            } catch (NumberFormatException ex) {
                return false;
            }
        }
        return true;
    }

    private static boolean isPrivateAddress(InetAddress address) {
        return address != null && (address.isLoopbackAddress() || address.isLinkLocalAddress() || address.isSiteLocalAddress());
    }

    private static boolean isPrivateHost(String url) {
        if (url == null || url.trim().isEmpty()) return false;
        try {
            Uri uri = Uri.parse(url.trim());
            String host = uri.getHost();
            if (host == null) return false;
            if ("localhost".equalsIgnoreCase(host) || "127.0.0.1".equals(host)) return true;
            return isPrivateAddress(InetAddress.getByName(host));
        } catch (Exception ex) {
            return false;
        }
    }

    private static String attribute(Map<String, byte[]> attrs, String key) {
        if (attrs == null) return "";
        byte[] value = attrs.get(key);
        return value == null ? "" : new String(value, StandardCharsets.UTF_8).trim();
    }

    private static String fingerprint(SslCertificate certificate) {
        if (certificate == null) return "";
        try {
            Bundle state = SslCertificate.saveState(certificate);
            byte[] der = state == null ? null : state.getByteArray("x509-certificate");
            if (der == null || der.length == 0) return "";
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(der);
            StringBuilder out = new StringBuilder(digest.length * 2);
            for (byte b : digest) out.append(String.format(Locale.ROOT, "%02X", b));
            return out.toString();
        } catch (Exception ex) {
            return "";
        }
    }

    private static String normalizeFingerprint(String value) {
        return value == null ? "" : value.replace(":", "").replace(" ", "").trim().toUpperCase(Locale.ROOT);
    }

    private static boolean validFingerprint(String value) {
        return value != null && value.matches("[0-9A-F]{64}");
    }

    private static String printable(String value) {
        return value == null || value.isEmpty() ? "—" : value;
    }

    private String tr(String german, String english) {
        return Locale.getDefault().getLanguage().equalsIgnoreCase("de") ? german : english;
    }

    private static String safeMessage(RuntimeException ex) {
        return ex == null || ex.getMessage() == null || ex.getMessage().trim().isEmpty()
                ? ex == null ? "" : ex.getClass().getSimpleName()
                : ex.getMessage().trim();
    }

    private void acquireMulticastLock() {
        if (multicastLock != null && multicastLock.isHeld()) return;
        WifiManager wifi = (WifiManager) getApplicationContext().getSystemService(Context.WIFI_SERVICE);
        if (wifi != null) {
            multicastLock = wifi.createMulticastLock("LocalCodeRemoteDiscovery");
            multicastLock.setReferenceCounted(false);
            multicastLock.acquire();
        }
    }

    private void releaseMulticastLock() {
        if (multicastLock != null && multicastLock.isHeld()) multicastLock.release();
        multicastLock = null;
    }

    private void stopDiscovery() {
        if (!discovering || nsdManager == null || discoveryListener == null) {
            releaseMulticastLock();
            return;
        }
        try {
            nsdManager.stopServiceDiscovery(discoveryListener);
        } catch (RuntimeException ignored) {
            discovering = false;
            releaseMulticastLock();
        }
    }

    private void setStatus(String message) {
        runOnUiThread(() -> {
            if (status != null) status.setText(message);
        });
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    @Override
    protected void onDestroy() {
        stopDiscovery();
        cancelPendingFileChooser();
        if (tts != null) {
            try {
                tts.stop();
                tts.shutdown();
            } catch (Exception ignored) {}
            tts = null;
        }
        if (webView != null) {
            webView.removeJavascriptInterface("LocalCodeAndroid");
            webView.stopLoading();
            webView.destroy();
        }
        super.onDestroy();
    }
}

