// SPDX-License-Identifier: Apache-2.0
package com.inetconnector.localcode.remote;

import android.Manifest;
import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.Bundle;
import android.net.http.SslCertificate;
import android.net.http.SslError;
import android.speech.RecognizerIntent;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.JavascriptInterface;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

import java.net.InetAddress;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Locale;
import java.util.Map;

import org.json.JSONObject;

@SuppressWarnings("deprecation")
public final class MainActivity extends Activity {
    private static final String SERVICE_TYPE = "_localcode._tcp.";
    private static final int REQUEST_NEARBY = 701;
    private static final int REQUEST_FILE_CHOOSER = 702;
    private static final int REQUEST_SPEECH = 703;

    private NsdManager nsdManager;
    private NsdManager.DiscoveryListener discoveryListener;
    private WifiManager.MulticastLock multicastLock;
    private WebView webView;
    private LinearLayout discoveryPanel;
    private TextView status;
    private EditText manualUrl;
    private EditText manualFingerprint;
    private ValueCallback<Uri[]> filePathCallback;
    private String expectedFingerprint = "";
    private String currentRemoteUrl = "";
    private boolean discovering;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        nsdManager = (NsdManager) getSystemService(Context.NSD_SERVICE);
        buildUi();
        handleIntent(getIntent());
        if (currentRemoteUrl.isEmpty()) {
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
        status.setText("Suche LocalCode im lokalen Netzwerk …");
        status.setPadding(0, dp(10), 0, dp(12));
        discoveryPanel.addView(status, fullWidthWrap());

        Button discover = new Button(this);
        discover.setText("LocalCode automatisch suchen");
        discover.setOnClickListener(v -> requestDiscoveryPermissionAndStart());
        discoveryPanel.addView(discover, fullWidthWrap());

        manualUrl = new EditText(this);
        manualUrl.setSingleLine(true);
        manualUrl.setHint("https://192.168.1.10:32146/remote");
        discoveryPanel.addView(manualUrl, fullWidthWrap());

        manualFingerprint = new EditText(this);
        manualFingerprint.setSingleLine(true);
        manualFingerprint.setHint("TLS SHA-256 Fingerprint vom LocalCode-PC");
        discoveryPanel.addView(manualFingerprint, fullWidthWrap());

        Button open = new Button(this);
        open.setText("Adresse sicher öffnen");
        open.setOnClickListener(v -> {
            String value = manualUrl.getText().toString().trim();
            String fp = normalizeFingerprint(manualFingerprint.getText().toString());
            if (isAllowedRemoteUrl(value) && validFingerprint(fp)) {
                expectedFingerprint = fp;
                openRemote(value);
            } else {
                setStatus("Manuell sind nur private HTTPS-IP-Adressen plus der 64-stellige TLS-SHA-256-Fingerprint vom LocalCode-PC erlaubt.");
            }
        });
        discoveryPanel.addView(open, fullWidthWrap());

        TextView qrHint = new TextView(this);
        qrHint.setText("Am einfachsten: QR-/Pair-Link verwenden. URL und TLS-Fingerprint werden dann automatisch und zusammen übernommen.");
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
                if (filePathCallback != null) {
                    filePathCallback.onReceiveValue(null);
                }
                filePathCallback = callback;
                Intent intent;
                try {
                    intent = params.createIntent();
                } catch (RuntimeException ex) {
                    filePathCallback = null;
                    setStatus("Dateiauswahl konnte nicht geöffnet werden: " + ex.getMessage());
                    return false;
                }
                try {
                    startActivityForResult(intent, REQUEST_FILE_CHOOSER);
                    return true;
                } catch (ActivityNotFoundException ex) {
                    filePathCallback = null;
                    setStatus("Keine passende Dateiauswahl-App gefunden.");
                    return false;
                }
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
                if (validFingerprint(expectedFingerprint) && expectedFingerprint.equalsIgnoreCase(observed)) {
                    handler.proceed();
                } else {
                    handler.cancel();
                    runOnUiThread(() -> {
                        webView.setVisibility(View.GONE);
                        discoveryPanel.setVisibility(View.VISIBLE);
                        setStatus("TLS-Zertifikat nicht bestätigt. Erwartet: " + printable(expectedFingerprint) + " · Empfangen: " + printable(observed));
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
                Uri[] results = resultCode == RESULT_OK ? WebChromeClient.FileChooserParams.parseResult(resultCode, data) : null;
                callback.onReceiveValue(results);
            }
            return;
        }
        if (requestCode == REQUEST_SPEECH) {
            if (resultCode == RESULT_OK && data != null) {
                ArrayList<String> matches = data.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS);
                if (matches != null && !matches.isEmpty()) {
                    deliverVoiceText(matches.get(0));
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
    }

    private void startVoiceRecognizer() {
        Intent intent = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
        intent.putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, false);
        intent.putExtra(RecognizerIntent.EXTRA_PROMPT, "LocalCode");
        try {
            startActivityForResult(intent, REQUEST_SPEECH);
        } catch (ActivityNotFoundException ex) {
            deliverVoiceText("");
            setStatus("Keine Spracheingabe-App gefunden.");
        }
    }

    private void deliverVoiceText(String text) {
        if (webView == null) return;
        String script = "window.localCodeVoiceResult&&window.localCodeVoiceResult(" + JSONObject.quote(text == null ? "" : text) + ")";
        webView.evaluateJavascript(script, null);
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
                setStatus("Lokale Netzwerkerkennung wurde nicht freigegeben. Alternativ private HTTPS-IP und TLS-Fingerprint vom LocalCode-PC manuell eingeben.");
            }
        }
    }

    private void startDiscovery() {
        if (nsdManager == null || discovering) {
            return;
        }
        acquireMulticastLock();
        setStatus("Suche LocalCode per mDNS …");
        discoveryListener = new NsdManager.DiscoveryListener() {
            @Override public void onDiscoveryStarted(String serviceType) { discovering = true; }
            @Override public void onDiscoveryStopped(String serviceType) { discovering = false; releaseMulticastLock(); }
            @Override public void onStartDiscoveryFailed(String serviceType, int errorCode) { discovering = false; releaseMulticastLock(); setStatus("mDNS-Suche konnte nicht gestartet werden (" + errorCode + ")."); }
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
            setStatus("mDNS-Suche fehlgeschlagen: " + ex.getMessage());
        }
    }

    private void resolve(NsdServiceInfo serviceInfo) {
        try {
            nsdManager.resolveService(serviceInfo, new NsdManager.ResolveListener() {
                @Override public void onResolveFailed(NsdServiceInfo info, int errorCode) { setStatus("LocalCode gefunden, Auflösung fehlgeschlagen (" + errorCode + ")."); }
                @Override public void onServiceResolved(NsdServiceInfo info) {
                    InetAddress host = info.getHost();
                    if (host == null || info.getPort() <= 0 || !isPrivateAddress(host)) {
                        setStatus("LocalCode-Dienst enthält keine verwendbare private LAN-Adresse.");
                        return;
                    }
                    Map<String, byte[]> attrs = info.getAttributes();
                    String tls = attribute(attrs, "tls");
                    String fp = normalizeFingerprint(attribute(attrs, "fp"));
                    String path = attribute(attrs, "path");
                    if (!"1".equals(tls) || !validFingerprint(fp)) {
                        setStatus("Unsicherer LocalCode-Dienst verworfen: gültiger TLS-Fingerprint fehlt.");
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
            setStatus("LocalCode-Auflösung fehlgeschlagen: " + ex.getMessage());
        }
    }

    private void handleIntent(Intent intent) {
        if (intent == null || intent.getData() == null) return;
        Uri data = intent.getData();
        if (!"localcode".equalsIgnoreCase(data.getScheme()) || !"pair".equalsIgnoreCase(data.getHost())) return;
        String target = data.getQueryParameter("url");
        String fp = normalizeFingerprint(data.getQueryParameter("fp"));
        if (target != null && isAllowedRemoteUrl(target) && validFingerprint(fp)) {
            expectedFingerprint = fp;
            openRemote(target);
        } else {
            setStatus("Der QR-/Deep-Link ist unvollständig oder unsicher.");
        }
    }

    private void openRemote(String target) {
        if (!isAllowedRemoteUrl(target) || !validFingerprint(expectedFingerprint)) {
            setStatus("Unsichere Remote-Adresse oder ungültiger TLS-Fingerprint verworfen.");
            return;
        }
        currentRemoteUrl = target;
        runOnUiThread(() -> {
            discoveryPanel.setVisibility(View.GONE);
            webView.setVisibility(View.VISIBLE);
            webView.loadUrl(target);
        });
    }

    private static boolean isAllowedRemoteUrl(String value) {
        if (value == null || value.trim().isEmpty()) return false;
        Uri uri = Uri.parse(value.trim());
        if (!"https".equalsIgnoreCase(uri.getScheme()) || uri.getHost() == null || uri.getUserInfo() != null) return false;
        String host = uri.getHost();
        if ("localhost".equalsIgnoreCase(host)) return true;
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
        runOnUiThread(() -> status.setText(message));
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    @Override
    protected void onDestroy() {
        stopDiscovery();
        if (webView != null) webView.destroy();
        super.onDestroy();
    }
}
