// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';
  // Keep the large localization dictionary isolated and parser-blocking so the
  // existing inline application script can use LocalCodeI18n immediately.
  document.write('<script src="/i18n_base.js"><\/script><script src="/ui_polish.js"><\/script><script src="/ui_approval_fix.js"><\/script>');
})();
