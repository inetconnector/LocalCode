// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';
  // Keep the large localization dictionary isolated and parser-blocking so the
  // existing inline application script can use LocalCodeI18n immediately.
  document.write('<script src="/i18n_base.js"><\/script><script src="/ui_polish.js"><\/script><script src="/ui_approval_fix.js"><\/script>');

  // Mission telemetry depends on the main inline application bindings (state,
  // api and renderInspector), so load it only after the document has completed.
  window.addEventListener('load', () => {
    if (document.querySelector('script[data-localcode-mission-status]')) return;
    const script = document.createElement('script');
    script.src = '/mission_status.js';
    script.dataset.localcodeMissionStatus = '1';
    document.head.appendChild(script);
  });
})();
