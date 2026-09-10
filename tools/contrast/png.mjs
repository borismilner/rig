// Minimal PNG decoder, for reading the pixels a browser actually painted.
//
// The gate needs pixel truth, not declared colour: `opacity` and `color-mix`
// composite at paint time, and a focus ring is not a text node at all, so no
// amount of walking the DOM finds either. Chrome hands back a PNG over CDP, and
// this turns it into RGBA bytes with no dependency to install in CI.
//
// Only what Chrome's Page.captureScreenshot emits is supported: bit depth 8,
// colour type 2 (RGB) or 6 (RGBA), no interlacing. Anything else throws rather
// than guessing, because a decoder that silently mis-reads is worse than none -
// every number downstream would be wrong and nothing would look broken.
import {inflateSync} from 'node:zlib';

const SIG = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

export function decodePng(buf) {
  if (buf.length < 8 || !buf.subarray(0, 8).equals(SIG)) throw new Error('png: bad signature');

  let off = 8, ihdr = null;
  const idat = [];
  while (off + 8 <= buf.length) {
    const len = buf.readUInt32BE(off);
    const type = buf.toString('latin1', off + 4, off + 8);
    const data = buf.subarray(off + 8, off + 8 + len);
    if (type === 'IHDR') {
      ihdr = {
        width: data.readUInt32BE(0), height: data.readUInt32BE(4),
        depth: data[8], colour: data[9], compression: data[10],
        filter: data[11], interlace: data[12],
      };
    } else if (type === 'IDAT') {
      idat.push(data);
    } else if (type === 'IEND') {
      break;
    }
    off += 12 + len;                 // length + type + data + crc
  }
  if (!ihdr) throw new Error('png: no IHDR');
  if (ihdr.depth !== 8) throw new Error(`png: bit depth ${ihdr.depth}, only 8 supported`);
  if (ihdr.colour !== 2 && ihdr.colour !== 6) throw new Error(`png: colour type ${ihdr.colour}, only 2 and 6 supported`);
  if (ihdr.interlace !== 0) throw new Error('png: interlaced, not supported');
  if (!idat.length) throw new Error('png: no IDAT');

  const {width, height} = ihdr;
  const chan = ihdr.colour === 6 ? 4 : 3;
  const raw = inflateSync(Buffer.concat(idat));
  const stride = width * chan;
  if (raw.length < (stride + 1) * height) throw new Error('png: short IDAT stream');

  // Unfilter in place, one scanline at a time. `prev` is the already-unfiltered
  // line above, which is what Up, Average and Paeth are defined against - using
  // the still-filtered bytes is the classic way to get a plausible-looking mess.
  const out = new Uint8Array(width * height * 4);
  let prev = new Uint8Array(stride);
  const line = new Uint8Array(stride);
  for (let y = 0; y < height; y++) {
    const p = y * (stride + 1);
    const ft = raw[p];
    raw.copy(line, 0, p + 1, p + 1 + stride);
    unfilter(ft, line, prev, chan, stride);
    for (let x = 0; x < width; x++) {
      const s = x * chan, d = (y * width + x) * 4;
      out[d] = line[s]; out[d + 1] = line[s + 1]; out[d + 2] = line[s + 2];
      out[d + 3] = chan === 4 ? line[s + 3] : 255;
    }
    prev = Uint8Array.prototype.slice.call(line);
  }
  return {width, height, data: out};
}

function unfilter(ft, line, prev, bpp, stride) {
  switch (ft) {
    case 0: return;
    case 1:
      for (let i = bpp; i < stride; i++) line[i] = (line[i] + line[i - bpp]) & 255;
      return;
    case 2:
      for (let i = 0; i < stride; i++) line[i] = (line[i] + prev[i]) & 255;
      return;
    case 3:
      for (let i = 0; i < stride; i++) {
        const a = i >= bpp ? line[i - bpp] : 0;
        line[i] = (line[i] + ((a + prev[i]) >> 1)) & 255;
      }
      return;
    case 4:
      for (let i = 0; i < stride; i++) {
        const a = i >= bpp ? line[i - bpp] : 0;
        const b = prev[i];
        const c = i >= bpp ? prev[i - bpp] : 0;
        const pa = Math.abs(b - c), pb = Math.abs(a - c), pc = Math.abs(a + b - 2 * c);
        const pr = pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
        line[i] = (line[i] + pr) & 255;
      }
      return;
    default:
      throw new Error(`png: unknown filter type ${ft}`);
  }
}

// x,y in image space; returns {r,g,b,a} with a in 0-1.
export function pixel(img, x, y) {
  const i = (y * img.width + x) * 4;
  return {r: img.data[i], g: img.data[i + 1], b: img.data[i + 2], a: img.data[i + 3] / 255};
}
