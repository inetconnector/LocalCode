// SPDX-License-Identifier: Apache-2.0
// Pure JavaScript micro QR Code Generator (Model 2, Byte Mode, SVG Output)
(() => {
  'use strict';

  // Minimal standard QR Code generator supporting byte payloads up to 150 bytes (V1..V6, EC-L / EC-M)
  function createQRCodeSVG(text, size = 240) {
    // Generate QR matrix
    const matrix = generateQRMatrix(text);
    const n = matrix.length;
    const padding = 4;
    const total = n + padding * 2;
    let rects = '';
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        if (matrix[r][c]) {
          rects += `<rect x="${c + padding}" y="${r + padding}" width="1" height="1" fill="#101215"/>`;
        }
      }
    }
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${total} ${total}" width="${size}" height="${size}" style="background:#fff;border-radius:12px;display:block;margin:0 auto;box-shadow:0 8px 30px rgba(0,0,0,0.25);">${rects}</svg>`;
  }

  // Polynomial Galois Field GF(256) arithmetic with primitive poly 0x11d (285)
  const GF_EXP = new Uint8Array(512);
  const GF_LOG = new Uint8Array(256);
  (() => {
    let x = 1;
    for (let i = 0; i < 255; i++) {
      GF_EXP[i] = x;
      GF_EXP[i + 255] = x;
      GF_LOG[x] = i;
      x <<= 1;
      if (x & 256) x ^= 0x11d;
    }
  })();

  function gfMul(x, y) {
    if (x === 0 || y === 0) return 0;
    return GF_EXP[GF_LOG[x] + GF_LOG[y]];
  }

  function polyMul(p, q) {
    const r = new Uint8Array(p.length + q.length - 1);
    for (let i = 0; i < p.length; i++) {
      for (let j = 0; j < q.length; j++) {
        r[i + j] ^= gfMul(p[i], q[j]);
      }
    }
    return r;
  }

  function polyMod(dividend, divisor) {
    const result = new Uint8Array(dividend);
    for (let i = 0; i <= result.length - divisor.length; i++) {
      const coef = result[i];
      if (coef !== 0) {
        for (let j = 1; j < divisor.length; j++) {
          result[i + j] ^= gfMul(divisor[j], coef);
        }
      }
    }
    return result.slice(result.length - divisor.length + 1);
  }

  function getGeneratorPoly(ecCount) {
    let g = new Uint8Array([1]);
    for (let i = 0; i < ecCount; i++) {
      g = polyMul(g, new Uint8Array([1, GF_EXP[i]]));
    }
    return g;
  }

  // Version table capacities (data bytes, EC bytes, total codewords) for EC Level L
  const VERSIONS = [
    null,
    { version: 1, size: 21, dataBytes: 19, ecBytes: 7, total: 26, align: [] },
    { version: 2, size: 25, dataBytes: 34, ecBytes: 10, total: 44, align: [6, 18] },
    { version: 3, size: 29, dataBytes: 55, ecBytes: 15, total: 70, align: [6, 22] },
    { version: 4, size: 33, dataBytes: 80, ecBytes: 20, total: 100, align: [6, 26] },
    { version: 5, size: 37, dataBytes: 108, ecBytes: 26, total: 134, align: [6, 30] },
    { version: 6, size: 41, dataBytes: 136, ecBytes: 18 * 2, total: 172, align: [6, 34] },
    { version: 7, size: 45, dataBytes: 156, ecBytes: 20 * 2, total: 196, align: [6, 22, 38] },
    { version: 8, size: 49, dataBytes: 194, ecBytes: 24 * 2, total: 242, align: [6, 24, 42] }
  ];

  function encodeData(text) {
    const bytes = new TextEncoder().encode(text);
    let vInfo = null;
    for (let v = 1; v < VERSIONS.length; v++) {
      const candidate = VERSIONS[v];
      // Mode (4 bits) + Count (8 bits for V1-9) + data bytes <= dataBytes
      if (bytes.length + 2 <= candidate.dataBytes) {
        vInfo = candidate;
        break;
      }
    }
    if (!vInfo) vInfo = VERSIONS[VERSIONS.length - 1];

    // Bit buffer
    const bits = [];
    const pushBits = (val, count) => {
      for (let i = count - 1; i >= 0; i--) {
        bits.push((val >> i) & 1);
      }
    };

    // Mode: Byte (0100)
    pushBits(0b0100, 4);
    // Character count (8 bits for byte mode in V1-9)
    pushBits(bytes.length, 8);
    for (let i = 0; i < bytes.length; i++) {
      pushBits(bytes[i], 8);
    }
    // Terminator up to 4 bits
    const maxDataBits = vInfo.dataBytes * 8;
    const termCount = Math.min(4, maxDataBits - bits.length);
    for (let i = 0; i < termCount; i++) bits.push(0);

    // Pad to byte boundary
    while (bits.length % 8 !== 0) bits.push(0);

    // Convert to bytes
    const dataCodewords = [];
    for (let i = 0; i < bits.length; i += 8) {
      let b = 0;
      for (let j = 0; j < 8; j++) b = (b << 1) | bits[i + j];
      dataCodewords.push(b);
    }

    // Pad bytes alternating 0xEC and 0x11
    const padBytes = [0xec, 0x11];
    let padIdx = 0;
    while (dataCodewords.length < vInfo.dataBytes) {
      dataCodewords.push(padBytes[padIdx % 2]);
      padIdx++;
    }

    // Reed Solomon Error Correction
    const gen = getGeneratorPoly(vInfo.ecBytes);
    const dividend = new Uint8Array(vInfo.total);
    dividend.set(dataCodewords, 0);
    const ecCodewords = polyMod(dividend, gen);

    const fullCodewords = new Uint8Array(vInfo.total);
    fullCodewords.set(dataCodewords, 0);
    fullCodewords.set(ecCodewords, vInfo.dataBytes);

    return { vInfo, codewords: fullCodewords };
  }

  function generateQRMatrix(text) {
    const { vInfo, codewords } = encodeData(text);
    const size = vInfo.size;
    const matrix = Array.from({ length: size }, () => Array(size).fill(null));
    const isReserved = Array.from({ length: size }, () => Array(size).fill(false));

    // 1. Finder patterns (7x7) + separators at (0,0), (0, size-7), (size-7, 0)
    function addFinder(top, left) {
      for (let r = -1; r <= 7; r++) {
        for (let c = -1; c <= 7; c++) {
          const row = top + r;
          const col = left + c;
          if (row >= 0 && row < size && col >= 0 && col < size) {
            isReserved[row][col] = true;
            if (r >= 0 && r <= 6 && c >= 0 && c <= 6) {
              matrix[row][col] = (r === 0 || r === 6 || c === 0 || c === 6 || (r >= 2 && r <= 4 && c >= 2 && c <= 4));
            } else {
              matrix[row][col] = false; // separator
            }
          }
        }
      }
    }
    addFinder(0, 0);
    addFinder(0, size - 7);
    addFinder(size - 7, 0);

    // 2. Alignment patterns
    if (vInfo.align && vInfo.align.length >= 2) {
      const coords = vInfo.align;
      for (const r of coords) {
        for (const c of coords) {
          if (isReserved[r][c]) continue;
          for (let dr = -2; dr <= 2; dr++) {
            for (let dc = -2; dc <= 2; dc++) {
              isReserved[r + dr][c + dc] = true;
              matrix[r + dr][c + dc] = (Math.abs(dr) === 2 || Math.abs(dc) === 2 || (dr === 0 && dc === 0));
            }
          }
        }
      }
    }

    // 3. Timing patterns (Row 6 and Col 6)
    for (let i = 0; i < size; i++) {
      if (!isReserved[6][i]) {
        isReserved[6][i] = true;
        matrix[6][i] = (i % 2 === 0);
      }
      if (!isReserved[i][6]) {
        isReserved[i][6] = true;
        matrix[i][6] = (i % 2 === 0);
      }
    }

    // 4. Dark module
    isReserved[size - 8][8] = true;
    matrix[size - 8][8] = true;

    // 5. Reserve format info areas (Row 8 and Col 8 near finders)
    for (let i = 0; i < 9; i++) {
      if (i < size) { isReserved[8][i] = true; isReserved[i][8] = true; }
    }
    for (let i = 0; i < 8; i++) {
      isReserved[size - 1 - i][8] = true;
      isReserved[8][size - 1 - i] = true;
    }

    // 6. Place data bits in 2-column zigzag
    const allBits = [];
    for (const b of codewords) {
      for (let i = 7; i >= 0; i--) allBits.push((b >> i) & 1);
    }
    let bitIdx = 0;
    let upwards = true;
    for (let col = size - 1; col > 0; col -= 2) {
      if (col === 6) col--; // Skip vertical timing line
      for (let i = 0; i < size; i++) {
        const row = upwards ? (size - 1 - i) : i;
        for (let c = 0; c < 2; c++) {
          const actualCol = col - c;
          if (!isReserved[row][actualCol]) {
            let bit = (bitIdx < allBits.length) ? allBits[bitIdx++] : 0;
            // Apply standard mask pattern 0 ( (row + col) % 2 == 0 )
            if ((row + actualCol) % 2 === 0) bit ^= 1;
            matrix[row][actualCol] = (bit === 1);
          }
        }
      }
      upwards = !upwards;
    }

    // 7. Write Format Information (EC Level L (01) + Mask 0 (000) -> Format bits 0b111011111000100)
    // Mask 0 with EC L format string (precomputed with BCH 15,5 and XOR 0x5412)
    const formatBits = [1, 1, 1, 0, 1, 1, 1, 1, 1, 0, 0, 0, 1, 0, 0];
    // Write horizontal format line
    let fIdx = 0;
    for (let c = 0; c <= 8; c++) {
      if (c !== 6) matrix[8][c] = !!formatBits[fIdx++];
    }
    for (let c = size - 8; c < size; c++) {
      matrix[8][c] = !!formatBits[fIdx++];
    }
    // Write vertical format line
    fIdx = 0;
    for (let r = size - 1; r >= size - 7; r--) {
      matrix[r][8] = !!formatBits[fIdx++];
    }
    matrix[size - 8][8] = true; // dark module
    for (let r = 8; r >= 0; r--) {
      if (r !== 6) matrix[r][8] = !!formatBits[fIdx++];
    }

    return matrix;
  }

  window.LocalCodeQR = {
    createQRCodeSVG
  };
})();
