type GlitchTextProps = {
  text: string;
  className?: string;
};

export default function GlitchText({ text, className }: GlitchTextProps) {
  return (
    <span className={className} data-glitch-text={text}>
      {text}
    </span>
  );
}
