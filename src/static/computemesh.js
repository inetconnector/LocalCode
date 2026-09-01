// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const i18n = window.LocalCodeI18n;
  if (i18n?.dictionaries) {
    Object.assign(i18n.dictionaries.de, {
      'ComputeMesh Integration': 'ComputeMesh Integration',
      'ComputeMesh': 'ComputeMesh',
      'ComputeMesh aktivieren': 'ComputeMesh aktivieren',
      'Verbindet LocalCode mit dem dezentralen ComputeMesh GPU-Cluster für 0% Plattformgebühr Provider Self-Compute.': 'Verbindet LocalCode mit dem dezentralen ComputeMesh GPU-Cluster für 0% Plattformgebühr Provider Self-Compute.',
      'Keys & Node automatisch erkennen': 'Keys & Node automatisch erkennen',
      'Liest Provider-Keys aus .computemesh oder Umgebungsvariablen und erkennt die lokale Workstation automatisch.': 'Liest Provider-Keys aus .computemesh oder Umgebungsvariablen und erkennt die lokale Workstation automatisch.',
      'ComputeMesh API-Key': 'ComputeMesh API-Key',
      'Persönlicher Provider-API-Key (z. B. cm_provider_...).': 'Persönlicher Provider-API-Key (z. B. cm_provider_...).',
      'ComputeMesh Gateway-URL': 'ComputeMesh Gateway-URL',
      'Standard: https://computemesh.inetconnector.com.': 'Standard: https://computemesh.inetconnector.com.',
      'Lokaler Workstation-Node': 'Lokaler Workstation-Node',
      'Lokale Node-Adresse (z. B. http://localhost:8080).': 'Lokale Node-Adresse (z. B. http://localhost:8080).',
      'Keys & Node jetzt einlesen': 'Keys & Node jetzt einlesen',
      'Verbindung testen': 'Verbindung testen',
      'ComputeMesh-Status wird geprüft …': 'ComputeMesh-Status wird geprüft …',
      'ComputeMesh ist online': 'ComputeMesh ist online',
      'ComputeMesh ist offline': 'ComputeMesh ist offline',
      'Gateway-Latenz': 'Gateway-Latenz',
      'Lokale Workstation': 'Lokale Workstation',
      'GPU & VRAM': 'GPU & VRAM',
      'Aktiver Key': 'Aktiver Key',
      'Key-Quelle': 'Key-Quelle',
      'Account': 'Account',
      'Verfügbare Cluster-Modelle': 'Verfügbare Cluster-Modelle',
      'Keys & Node werden eingelesen …': 'Keys & Node werden eingelesen …',
      'Keys erfolgreich erkannt und übernommen.': 'Keys erfolgreich erkannt und übernommen.',
      'Keine Keys in .computemesh oder Umgebungsvariablen gefunden.': 'Keine Keys in .computemesh oder Umgebungsvariablen gefunden.',
      'Verbindungstest wird ausgeführt …': 'Verbindungstest wird ausgeführt …',
      'Verbindungstest erfolgreich!': 'Verbindungstest erfolgreich!',
      'Verbindungstest fehlgeschlagen: ': 'Verbindungstest fehlgeschlagen: '
    });
    Object.assign(i18n.dictionaries.en, {
      'ComputeMesh Integration': 'ComputeMesh Integration',
      'ComputeMesh': 'ComputeMesh',
      'ComputeMesh aktivieren': 'Enable ComputeMesh',
      'Verbindet LocalCode mit dem dezentralen ComputeMesh GPU-Cluster für 0% Plattformgebühr Provider Self-Compute.': 'Connects LocalCode to the decentralized ComputeMesh GPU cluster for 0% platform fee Provider Self-Compute.',
      'Keys & Node automatisch erkennen': 'Auto-detect keys & node',
      'Liest Provider-Keys aus .computemesh oder Umgebungsvariablen und erkennt die lokale Workstation automatisch.': 'Reads provider keys from .computemesh or environment variables and detects the local workstation automatically.',
      'ComputeMesh API-Key': 'ComputeMesh API Key',
      'Persönlicher Provider-API-Key (z. B. cm_provider_...).': 'Personal provider API key (e.g. cm_provider_...).',
      'ComputeMesh Gateway-URL': 'ComputeMesh Gateway URL',
      'Standard: https://computemesh.inetconnector.com.': 'Default: https://computemesh.inetconnector.com.',
      'Lokaler Workstation-Node': 'Local Workstation Node',
      'Lokale Node-Adresse (z. B. http://localhost:8080).': 'Local node address (e.g. http://localhost:8080).',
      'Keys & Node jetzt einlesen': 'Auto-detect keys & node now',
      'Verbindung testen': 'Test connection',
      'ComputeMesh-Status wird geprüft …': 'Checking ComputeMesh status …',
      'ComputeMesh ist online': 'ComputeMesh is online',
      'ComputeMesh ist offline': 'ComputeMesh is offline',
      'Gateway-Latenz': 'Gateway latency',
      'Lokale Workstation': 'Local workstation',
      'GPU & VRAM': 'GPU & VRAM',
      'Aktiver Key': 'Active key',
      'Key-Quelle': 'Key source',
      'Account': 'Account',
      'Verfügbare Cluster-Modelle': 'Available cluster models',
      'Keys & Node werden eingelesen …': 'Auto-detecting keys & node …',
      'Keys erfolgreich erkannt und übernommen.': 'Keys detected and applied successfully.',
      'Keine Keys in .computemesh oder Umgebungsvariablen gefunden.': 'No keys found in .computemesh or environment variables.',
      'Verbindungstest wird ausgeführt …': 'Testing connection …',
      'Verbindungstest erfolgreich!': 'Connection test successful!',
      'Verbindungstest fehlgeschlagen: ': 'Connection test failed: '
    });
  }

  function tx(key) {
    return window.LocalCodeI18n ? window.LocalCodeI18n.t(key) : key;
  }

  async function checkComputeMeshStatus() {
    const out = document.getElementById('computeMeshResult');
    if (!out) return;
    out.classList.remove('hidden');
    out.textContent = tx('ComputeMesh-Status wird geprüft …');

    try {
      const res = await window.api('/api/computemesh/status');
      const st = res.status || {};
      const lines = [
        (st.online ? '🟢 ' : '🔴 ') + tx(st.online ? 'ComputeMesh ist online' : 'ComputeMesh ist offline'),
        'Gateway: ' + (st.url || 'https://computemesh.inetconnector.com') + (st.latency_ms ? ` (${st.latency_ms} ms)` : ''),
        tx('Lokale Workstation') + ': ' + (st.node_id || 'test-node-custom') + ' · ' + (st.node_status || 'Offline'),
        tx('GPU & VRAM') + ': ' + (st.gpu || 'NVIDIA RTX 3080') + (st.vram_pool ? ` · Pool: ${st.vram_pool}` : ''),
        tx('Account') + ': ' + (st.account || 'frede@inetconnector.com'),
        tx('Aktiver Key') + ': ' + (st.active_key_masked || 'cm_provider_…'),
        tx('Key-Quelle') + ': ' + (st.key_source || 'auto')
      ];

      if (st.models && st.models.length > 0) {
        lines.push('', tx('Verfügbare Cluster-Modelle') + ':');
        st.models.forEach(m => {
          lines.push(`  • ${m.name}`);
        });
      }

      if (st.error) {
        lines.push('', 'Fehler: ' + st.error);
      }

      out.textContent = lines.join('\n');
    } catch (e) {
      out.textContent = 'Fehler: ' + e.message;
    }
  }

  async function autoDetectComputeMesh() {
    const out = document.getElementById('computeMeshResult');
    if (!out) return;
    out.classList.remove('hidden');
    out.textContent = tx('Keys & Node werden eingelesen …');

    try {
      const res = await window.api('/api/computemesh/autodetect', { method: 'POST', body: '{}' });
      if (res.detected) {
        window.toast(tx('Keys erfolgreich erkannt und übernommen.'));
        if (window.state && window.state.settings) {
          window.state.settings.computemesh_enabled = true;
          window.state.settings.computemesh_auto_detect = true;
          if (res.status?.url) window.state.settings.computemesh_url = res.status.url;
        }
        if (typeof window.fillSettings === 'function') window.fillSettings();
      } else {
        window.toast(tx('Keine Keys in .computemesh oder Umgebungsvariablen gefunden.'), true);
      }
      await checkComputeMeshStatus();
    } catch (e) {
      out.textContent = 'Fehler: ' + e.message;
    }
  }

  async function testComputeMeshConnection() {
    const out = document.getElementById('computeMeshResult');
    if (!out) return;
    out.classList.remove('hidden');
    out.textContent = tx('Verbindungstest wird ausgeführt …');

    try {
      const res = await window.api('/api/computemesh/test', {
        method: 'POST',
        body: JSON.stringify({
          prompt: 'Hallo ComputeMesh! Bestätige bitte kurz deine GPU-Bereitschaft.',
          model: 'qwen/qwen2.5-7b-instruct'
        })
      });

      if (res.ok) {
        out.textContent = tx('Verbindungstest erfolgreich!') + '\n\n' +
          'Modell: ' + res.model + '\n' +
          'Gateway: ' + res.url + '\n\n' +
          'Antwort:\n' + res.response;
      } else {
        out.textContent = tx('Verbindungstest fehlgeschlagen: ') + (res.error || 'unbekannter Fehler');
      }
    } catch (e) {
      out.textContent = tx('Verbindungstest fehlgeschlagen: ') + e.message;
    }
  }

  window.addEventListener('DOMContentLoaded', () => {
    const statusBtn = document.getElementById('computeMeshStatusBtn');
    if (statusBtn) statusBtn.onclick = checkComputeMeshStatus;

    const autoBtn = document.getElementById('computeMeshAutoDetectBtn');
    if (autoBtn) autoBtn.onclick = autoDetectComputeMesh;

    const testBtn = document.getElementById('computeMeshTestBtn');
    if (testBtn) testBtn.onclick = testComputeMeshConnection;
  });
})();
