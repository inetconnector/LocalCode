'use strict';

const STRINGS = Object.freeze({
  en: Object.freeze({
    inlineEditPrompt: 'Describe the inline edit to apply to the selection',
    inlineEditPlaceholder: 'e.g. Extract helper function, add error handling, convert to async/await…',
    inlineEditProgress: 'LocalCode: Generating inline edit…',
    inlineEditDiffTitle: 'LocalCode Inline Edit Preview',
    inlineEditReviewPrompt: 'Inline edit ready for review. Apply changes to editor?',
    inlineEditAccept: 'Accept',
    inlineEditReject: 'Reject',
    inlineEditApplied: 'Inline edit applied.',
    inlineEditRejected: 'Inline edit discarded.',
    inlineEditNoPending: 'No pending inline edit to apply or discard.',
    inlineEditNoEditor: 'Open a local workspace file before starting an inline edit.',
    inlineEditEmptyResponse: 'The local model returned an empty edit.',
    inlineEditContextLimit: 'Inline edit context exceeds the 64 KiB limit. Select a smaller range.',
    inlineEditOutsideWorkspace: 'Inline edit is restricted to files inside the trusted workspace.',
    inlineEditInvalidEndpoint: 'Inline edit requires a local loopback HTTP endpoint (127.0.0.1, localhost, or [::1]).',
    inlineEditApplyFailed: 'Failed to apply the inline edit to the document.',
    inlineEditTimeout: 'Inline edit request timed out.',
    inlineEditTrustRequired: 'Trust this workspace before running inline edits.',
    fimEnabledNotice: 'Local FIM Tab autocomplete enabled.',
    fimDisabledNotice: 'Local FIM Tab autocomplete disabled.',
    fimInvalidEndpoint: 'FIM autocomplete requires a local loopback HTTP endpoint (127.0.0.1, localhost, or [::1]).',
    providerStatusTemplate: 'LocalCode Provider: {provider} | OS-Sandbox: {sandbox} | Configured keys: {count}'
  }),
  de: Object.freeze({
    inlineEditPrompt: 'Beschreibe die Inline-Änderung für die Auswahl',
    inlineEditPlaceholder: 'z. B. Hilfsfunktion extrahieren, Fehlerbehandlung ergänzen, auf async/await umstellen…',
    inlineEditProgress: 'LocalCode: Inline-Änderung wird erzeugt…',
    inlineEditDiffTitle: 'LocalCode Inline-Edit Vorschau',
    inlineEditReviewPrompt: 'Inline-Änderung zur Prüfung bereit. Änderungen im Editor übernehmen?',
    inlineEditAccept: 'Übernehmen',
    inlineEditReject: 'Verwerfen',
    inlineEditApplied: 'Inline-Änderung übernommen.',
    inlineEditRejected: 'Inline-Änderung verworfen.',
    inlineEditNoPending: 'Keine ausstehende Inline-Änderung zum Übernehmen oder Verwerfen.',
    inlineEditNoEditor: 'Öffne zuerst eine Datei im lokalen Arbeitsordner für einen Inline-Edit.',
    inlineEditEmptyResponse: 'Das lokale Modell hat keine Änderung zurückgegeben.',
    inlineEditContextLimit: 'Der Kontext für den Inline-Edit überschreitet das 64-KiB-Limit. Wähle einen kleineren Bereich.',
    inlineEditOutsideWorkspace: 'Inline-Edit ist auf Dateien innerhalb des vertrauenswürdigen Arbeitsordners beschränkt.',
    inlineEditInvalidEndpoint: 'Inline-Edit erfordert einen lokalen Loopback-HTTP-Endpunkt (127.0.0.1, localhost oder [::1]).',
    inlineEditApplyFailed: 'Die Inline-Änderung konnte nicht auf das Dokument angewendet werden.',
    inlineEditTimeout: 'Zeitlimit für die Inline-Edit-Anfrage überschritten.',
    inlineEditTrustRequired: 'Vertraue diesem Arbeitsordner, bevor du Inline-Edits ausführst.',
    fimEnabledNotice: 'Lokale FIM-Tab-Autovervollständigung aktiviert.',
    fimDisabledNotice: 'Lokale FIM-Tab-Autovervollständigung deaktiviert.',
    fimInvalidEndpoint: 'FIM-Autovervollständigung erfordert einen lokalen Loopback-HTTP-Endpunkt (127.0.0.1, localhost oder [::1]).',
    providerStatusTemplate: 'LocalCode-Provider: {provider} | OS-Sandbox: {sandbox} | Konfigurierte Keys: {count}'
  })
});

function resolveLanguage(preferred, candidates = []) {
  const pref = String(preferred || '').toLowerCase().trim();
  if (pref && pref !== 'auto') {
    return pref.startsWith('de') ? 'de' : 'en';
  }
  for (const cand of candidates) {
    if (cand && String(cand).toLowerCase().trim().startsWith('de')) {
      return 'de';
    }
  }
  return 'en';
}

function getStrings(lang) {
  return String(lang || '').toLowerCase().startsWith('de') ? STRINGS.de : STRINGS.en;
}

function t(lang, key) {
  const table = getStrings(lang);
  return Object.prototype.hasOwnProperty.call(table, key) ? table[key] : key;
}

function verifyParity() {
  const enKeys = Object.keys(STRINGS.en).sort();
  const deKeys = Object.keys(STRINGS.de).sort();
  if (enKeys.length !== deKeys.length) return false;
  for (let i = 0; i < enKeys.length; i++) {
    if (enKeys[i] !== deKeys[i]) return false;
  }
  return true;
}

module.exports = { STRINGS, resolveLanguage, getStrings, t, verifyParity };
