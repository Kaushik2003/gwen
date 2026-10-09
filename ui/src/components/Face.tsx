import type { Mood } from "../llm";
import { cx } from "./ui";

/** The assistant's sprites by path, one per mood. */
const sprites = import.meta.glob<string>("../assets/gwen/*.png", { eager: true, import: "default" });

/** The sprite of a mood, smiling for one this build has none of. */
export function faceUrl(mood: Mood): string {
  return sprites[`../assets/gwen/${mood}.png`] ?? sprites["../assets/gwen/smiling.png"];
}

/**
 * The assistant's face in a mood: a cut-out sticker, shown whole rather than
 * cropped to a circle. It breathes while idle; with react, it springs each
 * time the mood changes; talking, it bobs while she speaks, or with level
 * (0–1, how loud her voice is now), it moves with her voice instead.
 */
export default function Face({
  mood,
  react,
  talking,
  level,
  breathe = true,
  label,
  className,
}: {
  mood: Mood;
  react?: boolean;
  talking?: boolean;
  level?: number;
  breathe?: boolean;
  label?: string;
  className?: string;
}) {
  const voiced = level !== undefined;
  return (
    <span
      key={react ? mood : undefined}
      className={cx("inline-flex shrink-0 select-none", react && "animate-face", className)}
      aria-hidden={label ? undefined : true}
      role={label ? "img" : undefined}
      aria-label={label}
    >
      <span className={cx("inline-flex size-full origin-bottom", talking && !voiced ? "animate-talk" : breathe && !voiced && "animate-breathe")}>
        <img
          src={faceUrl(mood)}
          alt=""
          draggable={false}
          className="size-full origin-bottom object-contain transition-transform duration-100 ease-out"
          style={voiced ? { transform: `translateY(${-level * 7}%) scale(${1 + level * 0.12}) rotate(${(0.35 - level) * 4}deg)` } : undefined}
        />
      </span>
    </span>
  );
}
