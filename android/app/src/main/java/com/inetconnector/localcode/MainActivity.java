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
import android.view.GestureDetector;
import android.view.Gravity;
import android.view.MotionEvent;
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
import android.widget.ScrollView;
import android.widget.TextView;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
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
import java.util.concurrent.ConcurrentHashMap;
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
    private static final String PREF_LAST_PROJECT = "last_project";
    private static final int DEFAULT_REMOTE_PORT = 32146;
    private static final int DEFAULT_UDP_PORT = 32147;
    private static final int REQUEST_NEARBY = 701;
    private static final int REQUEST_FILE_CHOOSER = 702;
    private static final int REQUEST_SPEECH = 703;
    private static final int REQUEST_QR_SCAN = 704;

    public static final class DiscoveredInstance {
        final String hostname;
        final String version;
        final String url;
        final String fingerprint;
        final int port;
        final ArrayList<String> activeProjects = new ArrayList<>();
        final ArrayList<String> runningProjects = new ArrayList<>();
        long lastSeen;

        DiscoveredInstance(String hostname, String version, String url, String fingerprint, int port) {
            this.hostname = hostname == null ? "" : hostname.trim();
            this.version = version == null ? "" : version.trim();
            this.url = url == null ? "" : url.trim();
            this.fingerprint = fingerprint == null ? "" : fingerprint.trim();
            this.port = port;
            this.lastSeen = System.currentTimeMillis();
        }
    }

    private NsdManager nsdManager;
    private NsdManager.DiscoveryListener discoveryListener;
    private WifiManager.MulticastLock multicastLock;
    private WebView webView;
    private ScrollView discoveryPanel;
    private LinearLayout connectingOverlay;
    private TextView connectingStatus;
    private TextView connectingTarget;
    private LinearLayout instancesContainer;
    private LinearLayout tabAutoLayout;
    private LinearLayout tabQrLayout;
    private LinearLayout tabManualLayout;
    private Button tabAutoBtn;
    private Button tabQrBtn;
    private Button tabManualBtn;
    private final Map<String, DiscoveredInstance> discoveredInstances = new ConcurrentHashMap<>();
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
    private volatile boolean scanningUdp;
    private int activeWizardTab = 0;
    private GestureDetector gestureDetector;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O_MR1) {
            setShowWhenLocked(true);
            setTurnScreenOn(true);
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            getWindow().setStatusBarColor(0xFF0A0D14);
            getWindow().setNavigationBarColor(0xFF0A0D14);
        }
        gestureDetector = new GestureDetector(this, new GestureDetector.SimpleOnGestureListener() {
            @Override
            public boolean onFling(MotionEvent e1, MotionEvent e2, float velocityX, float velocityY) {
                if (e1 == null || e2 == null) return false;
                float diffX = e2.getX() - e1.getX();
                float diffY = e2.getY() - e1.getY();
                if (Math.abs(diffX) > Math.abs(diffY) && Math.abs(diffX) > dp(40) && Math.abs(velocityX) > 100) {
                    if (diffX < 0) {
                        switchToNextWizardTab();
                        return true;
                    } else {
                        switchToPrevWizardTab();
                        return true;
                    }
                }
                return false;
            }
        });
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
    public boolean dispatchTouchEvent(MotionEvent ev) {
        if (gestureDetector != null && discoveryPanel != null && discoveryPanel.getVisibility() == View.VISIBLE) {
            gestureDetector.onTouchEvent(ev);
        }
        return super.dispatchTouchEvent(ev);
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
        root.setBackgroundColor(0xFF0A0D14);
        root.setFitsSystemWindows(false);
        root.setLayoutParams(new ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        root.setOnApplyWindowInsetsListener((v, insets) -> {
            int top = 0;
            int bottom = 0;
            int left = 0;
            int right = 0;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                android.graphics.Insets bars = insets.getInsets(android.view.WindowInsets.Type.systemBars());
                top = bars.top;
                bottom = bars.bottom;
                left = bars.left;
                right = bars.right;
            } else {
                top = insets.getSystemWindowInsetTop();
                bottom = insets.getSystemWindowInsetBottom();
                left = insets.getSystemWindowInsetLeft();
                right = insets.getSystemWindowInsetRight();
            }
            int safeTop = Math.max(top, dp(24));
            int safeBottom = Math.max(bottom, dp(24));
            v.setPadding(left, safeTop, right, safeBottom);
            return insets;
        });

        discoveryPanel = new ScrollView(this);
        discoveryPanel.setBackgroundColor(0xFF0A0D14);
        discoveryPanel.setFillViewport(true);
        discoveryPanel.setLayoutParams(new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(22), dp(20), dp(22), dp(44));
        discoveryPanel.addView(content, fullWidthWrap());

        // 1. App Header / Branding
        LinearLayout brandRow = new LinearLayout(this);
        brandRow.setOrientation(LinearLayout.HORIZONTAL);
        brandRow.setGravity(Gravity.CENTER_VERTICAL);
        brandRow.setPadding(0, dp(8), 0, dp(20));

        TextView brandIcon = new TextView(this);
        brandIcon.setText("⚡");
        brandIcon.setTextSize(22f);
        brandIcon.setPadding(dp(10), dp(8), dp(10), dp(8));
        brandIcon.setBackground(createCardDrawable(0xFF1E293B, 0x3338BDF8, 14));
        brandRow.addView(brandIcon);

        LinearLayout brandTextCol = new LinearLayout(this);
        brandTextCol.setOrientation(LinearLayout.VERTICAL);
        brandTextCol.setPadding(dp(12), 0, 0, 0);

        TextView title = new TextView(this);
        title.setText("LocalCode Remote");
        title.setTextSize(22f);
        title.setTextColor(0xFFF8FAFC);
        title.setTypeface(null, android.graphics.Typeface.BOLD);
        brandTextCol.addView(title);

        TextView subTitle = new TextView(this);
        subTitle.setText(tr("Kabellose Desktop-Kopplung", "Wireless Desktop Pairing"));
        subTitle.setTextSize(13f);
        subTitle.setTextColor(0xFF94A3B8);
        brandTextCol.addView(subTitle);

        brandRow.addView(brandTextCol, fullWidthWrap());
        content.addView(brandRow, fullWidthWrap());

        // 2. Status Banner (Pulsing / Discovery Status)
        LinearLayout statusCard = new LinearLayout(this);
        statusCard.setOrientation(LinearLayout.HORIZONTAL);
        statusCard.setGravity(Gravity.CENTER_VERTICAL);
        statusCard.setPadding(dp(14), dp(10), dp(14), dp(10));
        statusCard.setBackground(createCardDrawable(0x1A38BDF8, 0x3338BDF8, 12));
        LinearLayout.LayoutParams statusLp = fullWidthWrap();
        statusLp.setMargins(0, 0, 0, dp(18));
        statusCard.setLayoutParams(statusLp);

        TextView statusDot = new TextView(this);
        statusDot.setText("📡 ");
        statusDot.setTextSize(14f);
        statusCard.addView(statusDot);

        status = new TextView(this);
        status.setText(tr("Suche LocalCode im lokalen Netzwerk …", "Searching for LocalCode on the local network …"));
        status.setTextSize(13f);
        status.setTextColor(0xFF38BDF8);
        status.setTypeface(null, android.graphics.Typeface.BOLD);
        statusCard.addView(status, fullWidthWrap());
        content.addView(statusCard);

        // 3. Modern 3-Tab Wizard Selector
        LinearLayout tabsRow = new LinearLayout(this);
        tabsRow.setOrientation(LinearLayout.HORIZONTAL);
        tabsRow.setPadding(dp(4), dp(4), dp(4), dp(4));
        tabsRow.setBackground(createCardDrawable(0xFF101522, 0x33475569, 14));
        LinearLayout.LayoutParams tabsLp = fullWidthWrap();
        tabsLp.setMargins(0, 0, 0, dp(18));
        tabsRow.setLayoutParams(tabsLp);

        tabAutoBtn = createTabButton(tr("📡 Auto-Suche", "📡 Auto-Find"), true);
        tabQrBtn = createTabButton(tr("📷 QR-Scan", "📷 QR Scan"), false);
        tabManualBtn = createTabButton(tr("⌨️ Manuell", "⌨️ Manual"), false);

        tabAutoBtn.setOnClickListener(v -> selectWizardTab(0));
        tabQrBtn.setOnClickListener(v -> selectWizardTab(1));
        tabManualBtn.setOnClickListener(v -> selectWizardTab(2));

        LinearLayout.LayoutParams tabItemLp = new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
        tabsRow.addView(tabAutoBtn, tabItemLp);
        tabsRow.addView(tabQrBtn, tabItemLp);
        tabsRow.addView(tabManualBtn, tabItemLp);
        content.addView(tabsRow);

        // 4. Tab 0: Auto Discovery Layout
        tabAutoLayout = new LinearLayout(this);
        tabAutoLayout.setOrientation(LinearLayout.VERTICAL);
        tabAutoLayout.setLayoutParams(fullWidthWrap());

        instancesContainer = new LinearLayout(this);
        instancesContainer.setOrientation(LinearLayout.VERTICAL);
        instancesContainer.setLayoutParams(fullWidthWrap());
        tabAutoLayout.addView(instancesContainer);
        content.addView(tabAutoLayout);

        // 5. Tab 1: QR Code Scanner Layout
        tabQrLayout = new LinearLayout(this);
        tabQrLayout.setOrientation(LinearLayout.VERTICAL);
        tabQrLayout.setPadding(dp(18), dp(18), dp(18), dp(18));
        tabQrLayout.setBackground(createCardDrawable(0xFF141A28, 0x33475569, 16));
        tabQrLayout.setLayoutParams(fullWidthWrap());
        tabQrLayout.setVisibility(View.GONE);

        TextView qrTitle = new TextView(this);
        qrTitle.setText(tr("📷 QR-Code vom PC scannen", "📷 Scan QR code from PC"));
        qrTitle.setTextSize(17f);
        qrTitle.setTextColor(0xFFF8FAFC);
        qrTitle.setTypeface(null, android.graphics.Typeface.BOLD);
        qrTitle.setPadding(0, 0, 0, dp(12));
        tabQrLayout.addView(qrTitle);

        TextView qrSteps = new TextView(this);
        qrSteps.setText(tr(
                "1. Öffne LocalCode auf deinem PC.\n2. Klicke links unten auf ⚙️ Einstellungen ➔ Remote.\n3. Klicke auf 'QR-Code anzeigen' und richte die Kamera darauf.",
                "1. Open LocalCode on your PC.\n2. Click ⚙️ Settings ➔ Remote at the bottom-left.\n3. Click 'Show QR Code' and point your camera at it."));
        qrSteps.setTextSize(13f);
        qrSteps.setTextColor(0xFFCBD5E1);
        qrSteps.setLineSpacing(dp(4), 1.2f);
        qrSteps.setPadding(0, 0, 0, dp(18));
        tabQrLayout.addView(qrSteps);

        Button startQrScanBtn = createActionButton(tr("📷 Kamera öffnen & QR scannen", "📷 Open camera & scan QR"), 0xFF7C3AED, 0xFFFFFFFF, v -> launchQrScanner());
        tabQrLayout.addView(startQrScanBtn, fullWidthWrap());
        content.addView(tabQrLayout);

        // 6. Tab 2: Manual Entry Layout
        tabManualLayout = new LinearLayout(this);
        tabManualLayout.setOrientation(LinearLayout.VERTICAL);
        tabManualLayout.setPadding(dp(18), dp(18), dp(18), dp(18));
        tabManualLayout.setBackground(createCardDrawable(0xFF141A28, 0x33475569, 16));
        tabManualLayout.setLayoutParams(fullWidthWrap());
        tabManualLayout.setVisibility(View.GONE);

        TextView manTitle = new TextView(this);
        manTitle.setText(tr("⌨️ Manuelle Verbindung", "⌨️ Manual Connection"));
        manTitle.setTextSize(17f);
        manTitle.setTextColor(0xFFF8FAFC);
        manTitle.setTypeface(null, android.graphics.Typeface.BOLD);
        manTitle.setPadding(0, 0, 0, dp(8));
        tabManualLayout.addView(manTitle);

        TextView manSub = new TextView(this);
        manSub.setText(tr("Gib die Remote-URL und optional den TLS-Fingerprint ein:", "Enter the remote URL and optional TLS fingerprint:"));
        manSub.setTextSize(13f);
        manSub.setTextColor(0xFF94A3B8);
        manSub.setPadding(0, 0, 0, dp(14));
        tabManualLayout.addView(manSub);

        manualUrl = createInputField("https://192.168.1.94:32146/remote");
        LinearLayout.LayoutParams urlLp = fullWidthWrap();
        urlLp.setMargins(0, 0, 0, dp(10));
        tabManualLayout.addView(manualUrl, urlLp);

        manualFingerprint = createInputField(tr("TLS-Fingerprint (optional bei HTTP)", "TLS fingerprint (optional for HTTP)"));
        LinearLayout.LayoutParams fpLp = fullWidthWrap();
        fpLp.setMargins(0, 0, 0, dp(16));
        tabManualLayout.addView(manualFingerprint, fpLp);

        Button open = createActionButton(tr("🚀 Manuell verbinden", "🚀 Connect manually"), 0xFF0284C7, 0xFFFFFFFF, null);
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
        tabManualLayout.addView(open, fullWidthWrap());
        content.addView(tabManualLayout);

        root.addView(discoveryPanel);

        // 7. Connecting Overlay (shown while WebView connects/loads)
        connectingOverlay = new LinearLayout(this);
        connectingOverlay.setOrientation(LinearLayout.VERTICAL);
        connectingOverlay.setGravity(Gravity.CENTER);
        connectingOverlay.setBackgroundColor(0xFF0A0D14);
        connectingOverlay.setPadding(dp(28), dp(40), dp(28), dp(40));
        connectingOverlay.setVisibility(View.GONE);

        TextView connIcon = new TextView(this);
        connIcon.setText("⚡");
        connIcon.setTextSize(36f);
        connIcon.setPadding(dp(18), dp(12), dp(18), dp(12));
        connIcon.setBackground(createCardDrawable(0xFF1E293B, 0x3338BDF8, 20));
        LinearLayout.LayoutParams connIconLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        connIconLp.setMargins(0, 0, 0, dp(18));
        connIcon.setLayoutParams(connIconLp);
        connectingOverlay.addView(connIcon);

        TextView connTitle = new TextView(this);
        connTitle.setText("LocalCode Remote");
        connTitle.setTextSize(22f);
        connTitle.setTextColor(0xFFF8FAFC);
        connTitle.setTypeface(null, android.graphics.Typeface.BOLD);
        connTitle.setGravity(Gravity.CENTER);
        connectingOverlay.addView(connTitle);

        connectingStatus = new TextView(this);
        connectingStatus.setText(tr("Verbinde mit Desktop-PC …", "Connecting to desktop PC …"));
        connectingStatus.setTextSize(14f);
        connectingStatus.setTextColor(0xFF38BDF8);
        connectingStatus.setTypeface(null, android.graphics.Typeface.BOLD);
        connectingStatus.setGravity(Gravity.CENTER);
        connectingStatus.setPadding(0, dp(8), 0, dp(4));
        connectingOverlay.addView(connectingStatus);

        connectingTarget = new TextView(this);
        connectingTarget.setText("");
        connectingTarget.setTextSize(12f);
        connectingTarget.setTextColor(0xFF94A3B8);
        connectingTarget.setGravity(Gravity.CENTER);
        connectingTarget.setPadding(0, 0, 0, dp(24));
        connectingOverlay.addView(connectingTarget);

        Button cancelConnBtn = createActionButton(tr("Zurück zur Suche", "Back to discovery"), 0xFF1E293B, 0xFFE2E8F0, v -> {
            cancelConnectingAndOpenDiscovery();
        });
        connectingOverlay.addView(cancelConnBtn, fullWidthWrap());

        root.addView(connectingOverlay, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        // 8. WebView
        webView = new WebView(this);
        webView.setBackgroundColor(0xFF0A0D14);
        configureWebView();
        webView.setVisibility(View.GONE);
        root.addView(webView, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        setContentView(root);
        renderDiscoveredInstances();
    }

    private void selectWizardTab(int index) {
        if (index < 0) index = 0;
        if (index > 2) index = 2;
        activeWizardTab = index;
        tabAutoBtn.setBackground(createTabButtonDrawable(index == 0));
        tabAutoBtn.setTextColor(index == 0 ? 0xFFFFFFFF : 0xFF94A3B8);

        tabQrBtn.setBackground(createTabButtonDrawable(index == 1));
        tabQrBtn.setTextColor(index == 1 ? 0xFFFFFFFF : 0xFF94A3B8);

        tabManualBtn.setBackground(createTabButtonDrawable(index == 2));
        tabManualBtn.setTextColor(index == 2 ? 0xFFFFFFFF : 0xFF94A3B8);

        tabAutoLayout.setVisibility(index == 0 ? View.VISIBLE : View.GONE);
        tabQrLayout.setVisibility(index == 1 ? View.VISIBLE : View.GONE);
        tabManualLayout.setVisibility(index == 2 ? View.VISIBLE : View.GONE);
    }

    private void switchToNextWizardTab() {
        int next = (activeWizardTab + 1) % 3;
        selectWizardTab(next);
        triggerHapticTick();
    }

    private void switchToPrevWizardTab() {
        int prev = (activeWizardTab - 1 + 3) % 3;
        selectWizardTab(prev);
        triggerHapticTick();
    }

    private void triggerHapticTick() {
        try {
            Vibrator v = (Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
            if (v != null && v.hasVibrator()) {
                if (Build.VERSION.SDK_INT >= 26) {
                    v.vibrate(VibrationEffect.createOneShot(15, VibrationEffect.DEFAULT_AMPLITUDE));
                } else {
                    v.vibrate(15);
                }
            }
        } catch (Exception ignored) {}
    }

    private Button createTabButton(String text, boolean active) {
        Button btn = new Button(this);
        btn.setText(text);
        btn.setTextSize(12.5f);
        btn.setTextColor(active ? 0xFFFFFFFF : 0xFF94A3B8);
        btn.setTypeface(null, android.graphics.Typeface.BOLD);
        btn.setBackground(createTabButtonDrawable(active));
        btn.setPadding(dp(8), dp(10), dp(8), dp(10));
        btn.setAllCaps(false);
        return btn;
    }

    private android.graphics.drawable.GradientDrawable createTabButtonDrawable(boolean active) {
        android.graphics.drawable.GradientDrawable gd = new android.graphics.drawable.GradientDrawable();
        if (active) {
            gd.setColor(0xFF2563EB);
            gd.setCornerRadius(dp(10));
        } else {
            gd.setColor(0x00000000);
            gd.setCornerRadius(dp(10));
        }
        return gd;
    }

    private Button createActionButton(String text, int bgColor, int textColor, View.OnClickListener onClick) {
        Button btn = new Button(this);
        btn.setText(text);
        btn.setTextSize(14f);
        btn.setTextColor(textColor);
        btn.setTypeface(null, android.graphics.Typeface.BOLD);
        btn.setBackground(createCardDrawable(bgColor, 0x00000000, 12));
        btn.setPadding(dp(16), dp(12), dp(16), dp(12));
        btn.setAllCaps(false);
        btn.setOnClickListener(onClick);
        return btn;
    }

    private EditText createInputField(String hint) {
        EditText et = new EditText(this);
        et.setSingleLine(true);
        et.setHint(hint);
        et.setHintTextColor(0xFF64748B);
        et.setTextColor(0xFFF1F5F9);
        et.setTextSize(13f);
        et.setPadding(dp(14), dp(12), dp(14), dp(12));
        et.setBackground(createCardDrawable(0xFF0C101A, 0x44475569, 12));
        return et;
    }

    private android.graphics.drawable.GradientDrawable createCardDrawable(int bgColor, int borderColor, int radiusDp) {
        android.graphics.drawable.GradientDrawable gd = new android.graphics.drawable.GradientDrawable();
        gd.setColor(bgColor);
        gd.setCornerRadius(dp(radiusDp));
        if (borderColor != 0) {
            gd.setStroke(dp(1), borderColor);
        }
        return gd;
    }

    private void renderDiscoveredInstances() {
        if (instancesContainer == null) return;
        instancesContainer.removeAllViews();

        if (discoveredInstances.isEmpty()) {
            LinearLayout emptyCard = new LinearLayout(this);
            emptyCard.setOrientation(LinearLayout.VERTICAL);
            emptyCard.setPadding(dp(18), dp(20), dp(18), dp(20));
            emptyCard.setBackground(createCardDrawable(0xFF141A28, 0x33475569, 16));
            emptyCard.setLayoutParams(fullWidthWrap());

            TextView emptyTitle = new TextView(this);
            emptyTitle.setText(tr("📡 Suche nach LocalCode-Instanzen …", "📡 Searching for LocalCode instances …"));
            emptyTitle.setTextSize(16f);
            emptyTitle.setTextColor(0xFFF8FAFC);
            emptyTitle.setTypeface(null, android.graphics.Typeface.BOLD);
            emptyTitle.setPadding(0, 0, 0, dp(8));
            emptyCard.addView(emptyTitle);

            TextView emptyDesc = new TextView(this);
            emptyDesc.setText(tr(
                    "LocalCode auf deinem PC gestartet? Sobald LocalCode läuft, wird die Instanz hier automatisch per UDP-Broadcast und mDNS angezeigt.",
                    "Is LocalCode running on your PC? Once started, the instance will appear here automatically via UDP broadcast and mDNS."));
            emptyDesc.setTextSize(13f);
            emptyDesc.setTextColor(0xFF94A3B8);
            emptyDesc.setLineSpacing(dp(4), 1.2f);
            emptyDesc.setPadding(0, 0, 0, dp(16));
            emptyCard.addView(emptyDesc);

            Button refreshBtn = createActionButton(tr("🔄 Suche neu starten", "🔄 Restart search"), 0xFF1E293B, 0xFFE2E8F0, v -> {
                requestDiscoveryPermissionAndStart();
            });
            emptyCard.addView(refreshBtn, fullWidthWrap());

            instancesContainer.addView(emptyCard);
            return;
        }

        TextView sectionHeader = new TextView(this);
        String headerTitle = tr("GEFUNDENE INSTANZEN (" + discoveredInstances.size() + ")",
                "DISCOVERED INSTANCES (" + discoveredInstances.size() + ")");
        sectionHeader.setText(headerTitle);
        sectionHeader.setTextSize(12f);
        sectionHeader.setTypeface(null, android.graphics.Typeface.BOLD);
        sectionHeader.setTextColor(0xFF64748B);
        sectionHeader.setPadding(dp(4), 0, 0, dp(10));
        instancesContainer.addView(sectionHeader, fullWidthWrap());

        for (DiscoveredInstance inst : discoveredInstances.values()) {
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            card.setPadding(dp(18), dp(18), dp(18), dp(18));
            card.setBackground(createCardDrawable(0xFF141A28, 0x33475569, 16));

            LinearLayout.LayoutParams lp = fullWidthWrap();
            lp.setMargins(0, 0, 0, dp(14));
            card.setLayoutParams(lp);

            // Row 1: Hostname + Version Badge
            LinearLayout topRow = new LinearLayout(this);
            topRow.setOrientation(LinearLayout.HORIZONTAL);
            topRow.setGravity(Gravity.CENTER_VERTICAL);

            TextView hostText = new TextView(this);
            String displayHost = inst.hostname.isEmpty() ? "LocalCode PC" : inst.hostname;
            hostText.setText("💻 " + displayHost);
            hostText.setTextSize(17f);
            hostText.setTextColor(0xFFF8FAFC);
            hostText.setTypeface(null, android.graphics.Typeface.BOLD);
            LinearLayout.LayoutParams hostLp = new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
            topRow.addView(hostText, hostLp);

            if (!inst.version.isEmpty()) {
                TextView verText = new TextView(this);
                verText.setText("v" + inst.version);
                verText.setTextSize(11f);
                verText.setTextColor(0xFF38BDF8);
                verText.setTypeface(null, android.graphics.Typeface.BOLD);
                verText.setPadding(dp(8), dp(3), dp(8), dp(3));
                verText.setBackground(createCardDrawable(0x2238BDF8, 0x3338BDF8, 8));
                topRow.addView(verText);
            }
            card.addView(topRow, fullWidthWrap());

            // Row 2: Address
            TextView addrText = new TextView(this);
            addrText.setText("🔗 " + inst.url);
            addrText.setTextSize(13f);
            addrText.setTextColor(0xFF94A3B8);
            addrText.setPadding(0, dp(6), 0, dp(8));
            card.addView(addrText, fullWidthWrap());

            // Row 3: Active / Running projects
            if (!inst.runningProjects.isEmpty()) {
                LinearLayout runBox = new LinearLayout(this);
                runBox.setOrientation(LinearLayout.HORIZONTAL);
                runBox.setPadding(dp(10), dp(6), dp(10), dp(6));
                runBox.setBackground(createCardDrawable(0x2216A34A, 0x4422C55E, 8));
                LinearLayout.LayoutParams runBoxLp = fullWidthWrap();
                runBoxLp.setMargins(0, 0, 0, dp(12));
                runBox.setLayoutParams(runBoxLp);

                TextView runText = new TextView(this);
                runText.setText("🟢 " + tr("Agent arbeitet an: ", "Agent working on: ") + String.join(", ", inst.runningProjects));
                runText.setTextSize(12f);
                runText.setTextColor(0xFF4ADE80);
                runText.setTypeface(null, android.graphics.Typeface.BOLD);
                runBox.addView(runText);
                card.addView(runBox);
            } else if (!inst.activeProjects.isEmpty()) {
                TextView projText = new TextView(this);
                projText.setText("📁 " + tr("Projekte: ", "Projects: ") + String.join(", ", inst.activeProjects));
                projText.setTextSize(12f);
                projText.setTextColor(0xFF94A3B8);
                projText.setPadding(0, 0, 0, dp(10));
                card.addView(projText, fullWidthWrap());
            }

            // 1-Click Connect button
            Button connectBtn = createActionButton(tr("⚡ 1-Klick Verbinden", "⚡ 1-Click Connect"), 0xFF2563EB, 0xFFFFFFFF, v -> {
                expectedFingerprint = inst.fingerprint;
                openRemote(inst.url);
            });
            card.addView(connectBtn, fullWidthWrap());

            instancesContainer.addView(card);
        }
    }

    private void startUdpBroadcastDiscovery() {
        if (scanningUdp) return;
        scanningUdp = true;
        new Thread(() -> {
            DatagramSocket socket = null;
            try {
                socket = new DatagramSocket();
                socket.setBroadcast(true);
                socket.setSoTimeout(3500);

                byte[] sendData = "LOCALCODE_DISCOVERY_PROBE".getBytes(StandardCharsets.UTF_8);

                // 1. Broadcast to 255.255.255.255:32147
                try {
                    DatagramPacket packet = new DatagramPacket(sendData, sendData.length, InetAddress.getByName("255.255.255.255"), DEFAULT_UDP_PORT);
                    socket.send(packet);
                } catch (Exception ignored) {}

                // 2. Broadcast to all interface broadcast addresses
                try {
                    for (NetworkInterface iface : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                        if (!iface.isUp() || iface.isLoopback()) continue;
                        for (InterfaceAddress ifAddr : iface.getInterfaceAddresses()) {
                            InetAddress bcast = ifAddr.getBroadcast();
                            if (bcast != null) {
                                try {
                                    DatagramPacket packet = new DatagramPacket(sendData, sendData.length, bcast, DEFAULT_UDP_PORT);
                                    socket.send(packet);
                                } catch (Exception ignored) {}
                            }
                        }
                    }
                } catch (Exception ignored) {}

                // 3. Receive responses
                byte[] recvBuf = new byte[8192];
                long endTime = System.currentTimeMillis() + 4500;
                while (System.currentTimeMillis() < endTime) {
                    DatagramPacket recvPacket = new DatagramPacket(recvBuf, recvBuf.length);
                    try {
                        socket.receive(recvPacket);
                        String jsonStr = new String(recvPacket.getData(), 0, recvPacket.getLength(), StandardCharsets.UTF_8);
                        JSONObject json = new JSONObject(jsonStr);
                        if (!json.optString("app", "").contains("LocalCode")) continue;

                        String hostname = json.optString("hostname", json.optString("instance_name", "LocalCode PC"));
                        String ver = json.optString("version", "");
                        int port = json.optInt("port", DEFAULT_REMOTE_PORT);
                        String fp = normalizeFingerprint(json.optString("tls_fingerprint", ""));
                        String peerHost = recvPacket.getAddress().getHostAddress();
                        String url = json.optString("url", "https://" + peerHost + ":" + port + "/remote");
                        if (!isAllowedRemoteUrl(url)) {
                            url = "https://" + peerHost + ":" + port + "/remote";
                        }
                        if (!isAllowedRemoteUrl(url)) continue;

                        DiscoveredInstance instance = new DiscoveredInstance(hostname, ver, url, fp, port);

                        JSONArray act = json.optJSONArray("active_projects");
                        if (act != null) {
                            for (int i = 0; i < act.length(); i++) {
                                String name = act.optString(i, "");
                                if (!name.isEmpty() && !instance.activeProjects.contains(name)) {
                                    instance.activeProjects.add(name);
                                }
                            }
                        }
                        JSONArray run = json.optJSONArray("running_projects");
                        if (run != null) {
                            for (int i = 0; i < run.length(); i++) {
                                String rp = run.optString(i, "");
                                String name = rp.contains("/") || rp.contains("\\") ? rp.substring(Math.max(rp.lastIndexOf('/'), rp.lastIndexOf('\\')) + 1) : rp;
                                if (!name.isEmpty() && !instance.runningProjects.contains(name)) {
                                    instance.runningProjects.add(name);
                                }
                            }
                        }

                        discoveredInstances.put(url, instance);
                        runOnUiThread(this::renderDiscoveredInstances);
                    } catch (Exception ex) {
                        if (System.currentTimeMillis() >= endTime) break;
                    }
                }
            } catch (Exception ignored) {
            } finally {
                if (socket != null && !socket.isClosed()) {
                    try { socket.close(); } catch (Exception ignored) {}
                }
                scanningUdp = false;
            }
        }, "UdpBroadcastDiscovery").start();
    }

    private LinearLayout.LayoutParams fullWidthWrap() {
        return new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
    }

    private void configureWebView() {
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSafeBrowsingEnabled(true);
        settings.setUseWideViewPort(true);
        settings.setLoadWithOverviewMode(true);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.KITKAT) {
            WebView.setWebContentsDebuggingEnabled(true);
        }
        webView.addJavascriptInterface(new AndroidBridge(), "LocalCodeAndroid");
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onProgressChanged(WebView view, int newProgress) {
                if (newProgress >= 85) {
                    runOnUiThread(() -> {
                        if (connectingOverlay != null) connectingOverlay.setVisibility(View.GONE);
                    });
                }
            }

            @Override
            public boolean onConsoleMessage(android.webkit.ConsoleMessage consoleMessage) {
                if (consoleMessage != null) {
                    android.util.Log.d("LocalCodeRemote", "JS: " + consoleMessage.message() + " (" + consoleMessage.sourceId() + ":" + consoleMessage.lineNumber() + ")");
                }
                return true;
            }

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
            public void onPageStarted(WebView view, String url, android.graphics.Bitmap favicon) {
                super.onPageStarted(view, url, favicon);
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                super.onPageFinished(view, url);
                runOnUiThread(() -> {
                    if (connectingOverlay != null) connectingOverlay.setVisibility(View.GONE);
                    if (webView != null) webView.setVisibility(View.VISIBLE);
                });
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
                    handleConnectionFailure(
                            "TLS-Zertifikat nicht bestätigt. Erwartet: " + printable(expectedFingerprint) + " · Empfangen: " + printable(observed),
                            "TLS certificate not confirmed. Expected: " + printable(expectedFingerprint) + " · Received: " + printable(observed));
                }
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request != null && request.isForMainFrame()) {
                    handleConnectionFailure(
                            "Gespeicherte Verbindung nicht erreichbar. Suche LocalCode erneut …",
                            "Saved connection is not reachable. Searching for LocalCode again …");
                }
            }

            @Override
            public void onReceivedError(WebView view, int errorCode, String description, String failingUrl) {
                handleConnectionFailure(
                        "Gespeicherte Verbindung nicht erreichbar (" + description + "). Suche LocalCode erneut …",
                        "Saved connection is not reachable (" + description + "). Searching for LocalCode again …");
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
        public void setLastProject(String path) {
            if (preferences != null && path != null) {
                preferences.edit().putString(PREF_LAST_PROJECT, path.trim()).apply();
            }
        }

        @JavascriptInterface
        public String getLastProject() {
            return preferences != null ? preferences.getString(PREF_LAST_PROJECT, "") : "";
        }

        @JavascriptInterface
        public void onAppReady() {
            runOnUiThread(() -> {
                if (connectingOverlay != null) {
                    connectingOverlay.setVisibility(View.GONE);
                }
            });
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
            return "2.2";
        }

        @JavascriptInterface
        public String getDeviceName() {
            String manufacturer = Build.MANUFACTURER;
            String model = Build.MODEL;
            if (model.toLowerCase(Locale.ROOT).startsWith(manufacturer.toLowerCase(Locale.ROOT))) {
                return model;
            }
            return manufacturer + " " + model;
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

        @JavascriptInterface
        public void unpair() {
            runOnUiThread(() -> {
                currentRemoteUrl = "";
                expectedFingerprint = "";
                clearSavedConnection();
                if (webView != null) {
                    webView.clearCache(true);
                    webView.setVisibility(View.GONE);
                }
                if (discoveryPanel != null) {
                    discoveryPanel.setVisibility(View.VISIBLE);
                    setStatus(tr(
                            "Gerät entkoppelt. Scanne den QR-Code auf dem Desktop-Bildschirm.",
                            "Device unpaired. Scan the QR code on your desktop screen."));
                }
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
        startUdpBroadcastDiscovery();
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
            ExecutorService pool = Executors.newFixedThreadPool(28);
            try {
                for (String host : lanProbeCandidates()) {
                    pool.execute(() -> {
                        probeLocalCode(host, port);
                    });
                }
                pool.shutdown();
                if (!pool.awaitTermination(7, TimeUnit.SECONDS)) pool.shutdownNow();
                runOnUiThread(() -> {
                    if (discoveredInstances.isEmpty()) {
                        setStatus(tr(
                                "Keine LocalCode-Instanz gefunden. QR-Code scannen oder Adresse vom PC eingeben.",
                                "No LocalCode instance found. Scan the QR code or enter the PC address."));
                    } else {
                        setStatus(tr(
                                discoveredInstances.size() + " LocalCode-Instanz(en) gefunden.",
                                discoveredInstances.size() + " LocalCode instance(s) found."));
                    }
                });
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
        addCandidateHost(out, seen, "192.168.1.94");
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
        result = probeLocalCodeEndpoint("http", host, port, false);
        if (result != null) return result;
        if (port != DEFAULT_REMOTE_PORT) {
            result = probeLocalCodeEndpoint("https", host, DEFAULT_REMOTE_PORT, true);
            if (result != null) return result;
            result = probeLocalCodeEndpoint("http", host, DEFAULT_REMOTE_PORT, false);
            if (result != null) return result;
        }
        if (port != 32145) {
            result = probeLocalCodeEndpoint("http", host, 32145, false);
            if (result != null) return result;
        }
        return null;
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
                if (!json.optString("app", "").contains("LocalCode")) continue;
                String fingerprint = normalizeFingerprint(json.optString("tls_fingerprint", ""));
                String target = discoveryURLFromJSON(json, scheme + "://" + host + ":" + port + "/remote");
                if (target == null || !isAllowedRemoteUrl(target)) continue;
                if ("https".equalsIgnoreCase(scheme) && path.endsWith("/discovery") && !validFingerprint(fingerprint)) continue;

                String hostname = json.optString("hostname", json.optString("instance_name", "LocalCode PC"));
                String version = json.optString("version", "");
                DiscoveredInstance instance = new DiscoveredInstance(hostname, version, target, fingerprint, port);

                JSONArray act = json.optJSONArray("active_projects");
                if (act != null) {
                    for (int i = 0; i < act.length(); i++) {
                        String name = act.optString(i, "");
                        if (!name.isEmpty() && !instance.activeProjects.contains(name)) {
                            instance.activeProjects.add(name);
                        }
                    }
                }
                JSONArray run = json.optJSONArray("running_projects");
                if (run != null) {
                    for (int i = 0; i < run.length(); i++) {
                        String rp = run.optString(i, "");
                        String name = rp.contains("/") || rp.contains("\\") ? rp.substring(Math.max(rp.lastIndexOf('/'), rp.lastIndexOf('\\')) + 1) : rp;
                        if (!name.isEmpty() && !instance.runningProjects.contains(name)) {
                            instance.runningProjects.add(name);
                        }
                    }
                }

                discoveredInstances.put(target, instance);
                runOnUiThread(this::renderDiscoveredInstances);
                return new ProbeResult(target, fingerprint, hostname, version);
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
        final String hostname;
        final String version;

        ProbeResult(String url, String fingerprint) {
            this(url, fingerprint, "", "");
        }

        ProbeResult(String url, String fingerprint, String hostname, String version) {
            this.url = url;
            this.fingerprint = fingerprint;
            this.hostname = hostname;
            this.version = version;
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
            if (discoveryPanel != null) discoveryPanel.setVisibility(View.GONE);
            if (connectingOverlay != null) {
                connectingOverlay.setVisibility(View.VISIBLE);
                if (connectingStatus != null) connectingStatus.setText(tr("Verbinde mit Desktop-PC …", "Connecting to desktop PC …"));
                if (connectingTarget != null) connectingTarget.setText(target);
            }
            if (webView != null) {
                webView.setVisibility(View.VISIBLE);
                webView.loadUrl(target);
            }
        });
    }

    private void cancelConnectingAndOpenDiscovery() {
        runOnUiThread(() -> {
            if (webView != null) {
                try { webView.stopLoading(); } catch (Exception ignored) {}
                webView.setVisibility(View.GONE);
            }
            if (connectingOverlay != null) {
                connectingOverlay.setVisibility(View.GONE);
            }
            if (discoveryPanel != null) {
                discoveryPanel.setVisibility(View.VISIBLE);
            }
            setStatus(tr("Suche LocalCode im lokalen Netzwerk …", "Searching for LocalCode on the local network …"));
            requestDiscoveryPermissionAndStart();
        });
    }

    private void handleConnectionFailure(String germanMsg, String englishMsg) {
        runOnUiThread(() -> {
            if (connectingOverlay != null) {
                connectingOverlay.setVisibility(View.GONE);
            }
            if (webView != null) {
                try { webView.stopLoading(); } catch (Exception ignored) {}
                webView.setVisibility(View.GONE);
            }
            if (discoveryPanel != null) {
                discoveryPanel.setVisibility(View.VISIBLE);
            }
            setStatus(tr(germanMsg, englishMsg));
            requestDiscoveryPermissionAndStart();
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

    private static String cleanRemoteBaseUrl(String target) {
        if (target == null) return "";
        int hashIdx = target.indexOf('#');
        if (hashIdx >= 0) target = target.substring(0, hashIdx);
        int queryIdx = target.indexOf('?');
        if (queryIdx >= 0) target = target.substring(0, queryIdx);
        return target.trim();
    }

    private void persistConnection(String target, String fingerprint) {
        String clean = cleanRemoteBaseUrl(target);
        if (preferences == null || !isAllowedRemoteUrl(clean)) return;
        preferences.edit()
                .putString(PREF_REMOTE_URL, clean)
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
            if (host == null || host.trim().isEmpty()) return false;
            host = host.trim().toLowerCase(Locale.ROOT);
            if ("localhost".equals(host) || "127.0.0.1".equals(host) || "::1".equals(host)) return true;
            if (host.startsWith("192.168.") || host.startsWith("10.") || host.startsWith("127.")) return true;
            if (host.matches("^172\\.(1[6-9]|2[0-9]|3[0-1])\\..*")) return true;
            if (host.endsWith(".local")) return true;
            return false;
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

