/** A hex colour at an opacity, as rgba(), for project colours on dark surfaces. */
export function alpha(hex: string, a: number): string {
  const h = hex.replace("#", "");
  const full = h.length === 3 ? h.replace(/./g, (c) => c + c) : h.slice(0, 6);
  const n = parseInt(full, 16);
  if (!Number.isFinite(n) || full.length !== 6) return `rgba(138, 143, 152, ${a})`;
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${a})`;
}

/** Colours offered for a new project, readable on the dark canvas. */
export const projectPalette = ["#5e6ad2", "#27a644", "#4ea7fc", "#f2c94c", "#eb5757", "#f2994a", "#bb87fc", "#4cb782", "#e46aa8", "#8a8f98"];

/** A hex colour blended at opacity a over a solid background, as an opaque rgb(). */
export function over(hex: string, a: number, background = "#141516"): string {
  const parse = (h: string) => {
    const s = h.replace("#", "");
    const full = s.length === 3 ? s.replace(/./g, (c) => c + c) : s.slice(0, 6);
    const n = parseInt(full, 16);
    return full.length === 6 && Number.isFinite(n) ? [(n >> 16) & 255, (n >> 8) & 255, n & 255] : [138, 143, 152];
  };
  const fg = parse(hex);
  const bg = parse(background);
  const mix = fg.map((c, i) => Math.round(c * a + bg[i] * (1 - a)));
  return `rgb(${mix.join(", ")})`;
}
