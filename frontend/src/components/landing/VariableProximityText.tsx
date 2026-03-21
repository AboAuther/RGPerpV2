import { useMemo, useState, type CSSProperties } from 'react';

type VariableProximityTextProps = {
  text: string;
  className?: string;
};

export default function VariableProximityText({ text, className }: VariableProximityTextProps) {
  const [pointerRatio, setPointerRatio] = useState<number | null>(null);
  const chars = useMemo(() => Array.from(text), [text]);

  return (
    <span
      className={className}
      onMouseMove={(event) => {
        const bounds = event.currentTarget.getBoundingClientRect();
        setPointerRatio((event.clientX - bounds.left) / Math.max(bounds.width, 1));
      }}
      onMouseLeave={() => setPointerRatio(null)}
    >
      {chars.map((char, index) => {
        const center = ((index + 0.5) / chars.length) * 100;
        const pointerPercent = pointerRatio === null ? null : pointerRatio * 100;
        const distance = pointerPercent === null ? 999 : Math.abs(center - pointerPercent);
        const influence = pointerPercent === null ? 0 : Math.max(0, 1 - distance / 18);
        const style = {
          transform: `translateY(${-(influence * 3)}px) scale(${1 + influence * 0.18})`,
          opacity: 0.72 + influence * 0.28,
          color: influence > 0.08 ? '#f4fbff' : undefined,
          textShadow: influence > 0.12 ? `0 0 ${12 + influence * 18}px rgba(80, 231, 209, 0.35)` : undefined,
        } satisfies CSSProperties;
        return (
          <span key={`${char}-${index}`} className="landing-variable-char" style={style}>
            {char === ' ' ? '\u00A0' : char}
          </span>
        );
      })}
    </span>
  );
}
