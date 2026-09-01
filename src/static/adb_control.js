// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const i18n = window.LocalCodeI18n;
  if (i18n?.dictionaries) {
    Object.assign(i18n.dictionaries.de, {
      'Android ADB': 'Android ADB',
      'Geräte suchen': 'Geräte suchen',
      'Auf Android ausführen': 'Auf Android ausführen',
      'App starten': 'App starten',
      'App beenden': 'App beenden',
      'Port 32145 spiegeln (Reverse)': 'Port 32145 spiegeln (Reverse)',
      'Logcat anzeigen': 'Logcat anzeigen',
      'Kein Android-Gerät über ADB verbunden.': 'Kein Android-Gerät über ADB verbunden.',
      '1 Android-Gerät verbunden: ': '1 Android-Gerät verbunden: ',
      'Android-Geräte verbunden: ': 'Android-Geräte verbunden: ',
      'APK wird gebaut und auf dem Smartphone gestartet …': 'APK wird gebaut und auf dem Smartphone gestartet …',
      'Erfolgreich auf Smartphone übertragen und gestartet!': 'Erfolgreich auf Smartphone übertragen und gestartet!',
      'Fehler bei der ADB-Übertragung: ': 'Fehler bei der ADB-Übertragung: ',
      'Port 32145 erfolgreich auf dem Smartphone gespiegelt (http://localhost:32145 aktiv).': 'Port 32145 erfolgreich auf dem Smartphone gespiegelt (http://localhost:32145 aktiv).',
      'App erfolgreich beendet.': 'App erfolgreich beendet.'
    });
    Object.assign(i18n.dictionaries.en, {
      'Android ADB': 'Android ADB',
      'Geräte suchen': 'Scan Devices',
      'Auf Android ausführen': 'Deploy to Android',
      'App starten': 'Launch App',
      'App beenden': 'Force Stop App',
      'Port 32145 spiegeln (Reverse)': 'Reverse Port 32145',
      'Logcat anzeigen': 'Show Logcat',
      'Kein Android-Gerät über ADB verbunden.': 'No Android device connected via ADB.',
      '1 Android-Gerät verbunden: ': '1 Android device connected: ',
      'Android-Geräte verbunden: ': 'Android devices connected: ',
      'APK wird gebaut und auf dem Smartphone gestartet …': 'Building APK and launching on smartphone …',
      'Erfolgreich auf Smartphone übertragen und gestartet!': 'Successfully deployed and launched on smartphone!',
      'Fehler bei der ADB-Übertragung: ': 'ADB deployment error: ',
      'Port 32145 erfolgreich auf dem Smartphone gespiegelt (http://localhost:32145 aktiv).': 'Port 32145 successfully reversed to smartphone (http://localhost:32145 active).',
      'App erfolgreich beendet.': 'App stopped successfully.'
    });
  }

  function tx(key) {
    return window.LocalCodeI18n ? window.LocalCodeI18n.t(key) : key;
  }

  async function checkADBDevices() {
    try {
      const res = await window.api('/api/adb/devices');
      const devs = res.devices || [];
      const badge = document.getElementById('adbDeviceBadge');
      if (!badge) return devs;
      if (devs.length === 0) {
        badge.innerHTML = '<span class="status-dot offline"></span>' + tx('Kein Android-Gerät über ADB verbunden.');
        badge.classList.remove('active');
      } else {
        const d = devs[0];
        const label = d.Serial + (d.Line ? ` (${d.Line.split(' ')[0]})` : '');
        badge.innerHTML = `<span class="status-dot ready"></span>📱 ${d.Serial}`;
        badge.title = devs.map(x => x.Line).join('\n');
        badge.classList.add('active');
      }
      return devs;
    } catch (e) {
      return [];
    }
  }

  async function deployToAndroid() {
    const toast = document.getElementById('toast') || alert;
    try {
      const btn = document.getElementById('adbDeployBtn');
      if (btn) btn.disabled = true;
      const res = await window.api('/api/adb/deploy', { method: 'POST' });
      if (res.ok) {
        if (typeof showToast === 'function') showToast(tx('Erfolgreich auf Smartphone übertragen und gestartet!'));
      } else {
        if (typeof showToast === 'function') showToast(tx('Fehler bei der ADB-Übertragung: ') + (res.error || 'Unknown error'));
      }
    } catch (e) {
      if (typeof showToast === 'function') showToast(tx('Fehler bei der ADB-Übertragung: ') + e.message);
    } finally {
      const btn = document.getElementById('adbDeployBtn');
      if (btn) btn.disabled = false;
    }
  }

  async function reversePort32145() {
    try {
      const res = await window.api('/api/adb/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'reverse', target: '32145' })
      });
      if (res.ok) {
        if (typeof showToast === 'function') showToast(tx('Port 32145 erfolgreich auf dem Smartphone gespiegelt (http://localhost:32145 aktiv).'));
      }
    } catch (e) {
      if (typeof showToast === 'function') showToast('Reverse error: ' + e.message);
    }
  }

  window.LocalCodeADB = {
    checkDevices: checkADBDevices,
    deploy: deployToAndroid,
    reverse: reversePort32145
  };

  window.addEventListener('load', () => {
    setInterval(checkADBDevices, 15000);
    setTimeout(checkADBDevices, 1500);
  });
})();
