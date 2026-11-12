// A QR code encoder: text in, a square of dark and light modules out.
// Byte mode with error correction level M, which is what a link needs.
// The steps and tables follow ISO/IEC 18004, as the `qrcode` npm package
// (MIT) lays them out, and give the same picture as it does for the same text.

/** Codewords in a symbol of each version (index 1 to 40). */
const TOTAL = [
  0, 26, 44, 70, 100, 134, 172, 196, 242, 292, 346, 404, 466, 532, 581, 655, 733, 815, 901, 991, 1085, 1156, 1258, 1364, 1474, 1588, 1706, 1828, 1921, 2051, 2185, 2323, 2465, 2611, 2761, 2876, 3034, 3196,
  3362, 3532, 3706,
];
/** Error correction blocks at level M, per version. */
const BLOCKS = [0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49];
/** Error correction codewords at level M, per version. */
const EC = [
  0, 10, 16, 26, 36, 48, 64, 72, 88, 110, 130, 150, 176, 198, 216, 240, 280, 308, 338, 364, 416, 442, 476, 504, 560, 588, 644, 700, 728, 784, 812, 868, 924, 980, 1036, 1064, 1120, 1204, 1260, 1316, 1372,
];

// GF(256) with the QR polynomial x^8 + x^4 + x^3 + x^2 + 1.
const EXP = new Uint8Array(512);
const LOG = new Uint8Array(256);
for (let i = 0, x = 1; i < 255; i++) {
  EXP[i] = x;
  LOG[x] = i;
  x <<= 1;
  if (x & 0x100) x ^= 0x11d;
}
for (let i = 255; i < 512; i++) EXP[i] = EXP[i - 255];
const mul = (a, b) => (a === 0 || b === 0 ? 0 : EXP[LOG[a] + LOG[b]]);

/** The Reed-Solomon error correction codewords of data. */
function reedSolomon(data, count) {
  let gen = [1];
  for (let i = 0; i < count; i++) {
    const next = new Array(gen.length + 1).fill(0);
    for (let j = 0; j < gen.length; j++) {
      next[j] ^= gen[j];
      next[j + 1] ^= mul(gen[j], EXP[i]);
    }
    gen = next;
  }
  const rem = new Array(count).fill(0);
  for (const byte of data) {
    const factor = byte ^ rem.shift();
    rem.push(0);
    if (factor !== 0) for (let j = 0; j < count; j++) rem[j] ^= mul(gen[j + 1], factor);
  }
  return rem;
}

/** Text as UTF-8 bytes. */
function utf8(text) {
  const out = [];
  for (const ch of String(text)) {
    const c = ch.codePointAt(0);
    if (c < 0x80) out.push(c);
    else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
    else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
    else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
  }
  return out;
}

/** How many bytes a version holds in byte mode at level M. */
const capacity = (version) => Math.floor(((TOTAL[version] - EC[version]) * 8 - 4 - (version < 10 ? 8 : 16)) / 8);

/** The data and error correction codewords of bytes, in the order they are drawn. */
function codewords(bytes, version) {
  const dataTotal = TOTAL[version] - EC[version];
  const bits = [];
  const put = (value, length) => {
    for (let i = length - 1; i >= 0; i--) bits.push((value >>> i) & 1);
  };
  put(4, 4); // byte mode
  put(bytes.length, version < 10 ? 8 : 16);
  for (const b of bytes) put(b, 8);
  if (bits.length + 4 <= dataTotal * 8) put(0, 4);
  while (bits.length % 8 !== 0) bits.push(0);
  const data = [];
  for (let i = 0; i < bits.length; i += 8) data.push(bits.slice(i, i + 8).reduce((n, bit) => (n << 1) | bit, 0));
  for (let i = 0; data.length < dataTotal; i++) data.push(i % 2 ? 0x11 : 0xec);

  // Split into blocks (the later ones a codeword longer), correct each, then interleave.
  const blocks = BLOCKS[version];
  const longer = TOTAL[version] % blocks;
  const short = Math.floor(dataTotal / blocks);
  const ecCount = Math.floor(TOTAL[version] / blocks) - short;
  const dc = [];
  const ec = [];
  let offset = 0;
  for (let b = 0; b < blocks; b++) {
    const size = short + (b < blocks - longer ? 0 : 1);
    dc.push(data.slice(offset, offset + size));
    ec.push(reedSolomon(dc[b], ecCount));
    offset += size;
  }
  const out = [];
  for (let i = 0; i <= short; i++) for (const block of dc) if (i < block.length) out.push(block[i]);
  for (let i = 0; i < ecCount; i++) for (const block of ec) out.push(block[i]);
  return out;
}

/** Where the small alignment squares sit along one side. */
function alignmentCoords(version) {
  if (version === 1) return [];
  const count = Math.floor(version / 7) + 2;
  const size = version * 4 + 17;
  const step = size === 145 ? 26 : Math.ceil((size - 13) / (2 * count - 2)) * 2;
  const out = [size - 7];
  for (let i = 1; i < count - 1; i++) out[i] = out[i - 1] - step;
  out.push(6);
  return out.reverse();
}

const bchDigits = (n) => (n === 0 ? 0 : 32 - Math.clz32(n));

/** The 15 format bits for level M and a mask. */
function formatBits(mask) {
  const G15 = 0x537;
  const data = mask; // level M is 00
  let d = data << 10;
  while (bchDigits(d) - bchDigits(G15) >= 0) d ^= G15 << (bchDigits(d) - bchDigits(G15));
  return ((data << 10) | d) ^ 0x5412;
}

/** The 18 version bits, drawn from version 7 up. */
function versionBits(version) {
  const G18 = 0x1f25;
  let d = version << 12;
  while (bchDigits(d) - bchDigits(G18) >= 0) d ^= G18 << (bchDigits(d) - bchDigits(G18));
  return (version << 12) | d;
}

const maskAt = (mask, i, j) => {
  switch (mask) {
    case 0:
      return (i + j) % 2 === 0;
    case 1:
      return i % 2 === 0;
    case 2:
      return j % 3 === 0;
    case 3:
      return (i + j) % 3 === 0;
    case 4:
      return (Math.floor(i / 2) + Math.floor(j / 3)) % 2 === 0;
    case 5:
      return ((i * j) % 2) + ((i * j) % 3) === 0;
    case 6:
      return (((i * j) % 2) + ((i * j) % 3)) % 2 === 0;
    default:
      return (((i * j) % 3) + ((i + j) % 2)) % 2 === 0;
  }
};

/** How hard a finished symbol is to read: lower is better. */
function penalty(m, size) {
  let points = 0;
  const at = (r, c) => m[r * size + c];
  // Runs of five or more of one colour.
  for (let row = 0; row < size; row++) {
    let runCol = 0;
    let runRow = 0;
    let lastCol = -1;
    let lastRow = -1;
    for (let col = 0; col < size; col++) {
      let v = at(row, col);
      if (v === lastCol) runCol++;
      else {
        if (runCol >= 5) points += 3 + (runCol - 5);
        lastCol = v;
        runCol = 1;
      }
      v = at(col, row);
      if (v === lastRow) runRow++;
      else {
        if (runRow >= 5) points += 3 + (runRow - 5);
        lastRow = v;
        runRow = 1;
      }
    }
    if (runCol >= 5) points += 3 + (runCol - 5);
    if (runRow >= 5) points += 3 + (runRow - 5);
  }
  // Two by two blocks of one colour.
  for (let row = 0; row < size - 1; row++)
    for (let col = 0; col < size - 1; col++) {
      const sum = at(row, col) + at(row, col + 1) + at(row + 1, col) + at(row + 1, col + 1);
      if (sum === 4 || sum === 0) points += 3;
    }
  // Runs that look like a finder pattern.
  for (let row = 0; row < size; row++) {
    let bitsCol = 0;
    let bitsRow = 0;
    for (let col = 0; col < size; col++) {
      bitsCol = ((bitsCol << 1) & 0x7ff) | at(row, col);
      if (col >= 10 && (bitsCol === 0x5d0 || bitsCol === 0x05d)) points += 40;
      bitsRow = ((bitsRow << 1) & 0x7ff) | at(col, row);
      if (col >= 10 && (bitsRow === 0x5d0 || bitsRow === 0x05d)) points += 40;
    }
  }
  // Far from half dark.
  let dark = 0;
  for (let i = 0; i < m.length; i++) dark += m[i];
  points += Math.abs(Math.ceil((dark * 100) / m.length / 5) - 10) * 10;
  return points;
}

/**
 * The QR code of text as rows of 0 (light) and 1 (dark), without the white
 * border a reader needs around it. Throws when the text is too long for the
 * largest symbol (about 2,300 bytes).
 */
export function qrMatrix(text) {
  const bytes = utf8(text);
  let version = 1;
  while (version <= 40 && bytes.length > capacity(version)) version++;
  if (version > 40) throw new Error("The text is too long for a QR code");
  const size = version * 4 + 17;
  const m = new Uint8Array(size * size);
  const reserved = new Uint8Array(size * size);
  const set = (row, col, dark, keep) => {
    m[row * size + col] = dark ? 1 : 0;
    if (keep) reserved[row * size + col] = 1;
  };

  // The three finder squares with their white margins.
  for (const [row, col] of [
    [0, 0],
    [size - 7, 0],
    [0, size - 7],
  ])
    for (let r = -1; r <= 7; r++) {
      if (row + r < 0 || row + r >= size) continue;
      for (let c = -1; c <= 7; c++) {
        if (col + c < 0 || col + c >= size) continue;
        const dark = (r >= 0 && r <= 6 && (c === 0 || c === 6)) || (c >= 0 && c <= 6 && (r === 0 || r === 6)) || (r >= 2 && r <= 4 && c >= 2 && c <= 4);
        set(row + r, col + c, dark, true);
      }
    }
  // The timing lines between them.
  for (let i = 8; i < size - 8; i++) {
    set(i, 6, i % 2 === 0, true);
    set(6, i, i % 2 === 0, true);
  }
  // The alignment squares, except where a finder is.
  const coords = alignmentCoords(version);
  const last = coords.length - 1;
  for (let i = 0; i <= last; i++)
    for (let j = 0; j <= last; j++) {
      if ((i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0)) continue;
      for (let r = -2; r <= 2; r++) for (let c = -2; c <= 2; c++) set(coords[i] + r, coords[j] + c, Math.max(Math.abs(r), Math.abs(c)) !== 1, true);
    }
  const drawFormat = (mask) => {
    const bits = formatBits(mask);
    for (let i = 0; i < 15; i++) {
      const dark = ((bits >> i) & 1) === 1;
      if (i < 6) set(i, 8, dark, true);
      else if (i < 8) set(i + 1, 8, dark, true);
      else set(size - 15 + i, 8, dark, true);
      if (i < 8) set(8, size - i - 1, dark, true);
      else if (i < 9) set(8, 15 - i, dark, true);
      else set(8, 15 - i - 1, dark, true);
    }
    set(size - 8, 8, true, true);
  };
  drawFormat(0);
  if (version >= 7) {
    const bits = versionBits(version);
    for (let i = 0; i < 18; i++) {
      const row = Math.floor(i / 3);
      const col = (i % 3) + size - 11;
      const dark = ((bits >> i) & 1) === 1;
      set(row, col, dark, true);
      set(col, row, dark, true);
    }
  }

  // The codewords, in two-module columns from the bottom right, going up then down.
  const data = codewords(bytes, version);
  let inc = -1;
  let row = size - 1;
  let bit = 7;
  let byte = 0;
  for (let col = size - 1; col > 0; col -= 2) {
    if (col === 6) col--;
    for (;;) {
      for (let c = 0; c < 2; c++) {
        if (reserved[row * size + col - c]) continue;
        set(row, col - c, byte < data.length && ((data[byte] >>> bit) & 1) === 1);
        if (--bit === -1) {
          byte++;
          bit = 7;
        }
      }
      row += inc;
      if (row < 0 || row >= size) {
        row -= inc;
        inc = -inc;
        break;
      }
    }
  }

  // The mask that leaves the most readable picture.
  const applyMask = (mask) => {
    for (let r = 0; r < size; r++) for (let c = 0; c < size; c++) if (!reserved[r * size + c] && maskAt(mask, r, c)) m[r * size + c] ^= 1;
  };
  let best = 0;
  let lowest = Infinity;
  for (let mask = 0; mask < 8; mask++) {
    drawFormat(mask);
    applyMask(mask);
    const p = penalty(m, size);
    applyMask(mask);
    if (p < lowest) {
      lowest = p;
      best = mask;
    }
  }
  applyMask(best);
  drawFormat(best);

  const rows = [];
  for (let r = 0; r < size; r++) rows.push(Array.from(m.subarray(r * size, (r + 1) * size)));
  return rows;
}
