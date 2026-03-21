type ShinyTextProps = {
  text: string;
  className?: string;
};

export default function ShinyText({ text, className }: ShinyTextProps) {
  return (
    <span className={className} data-shiny-text>
      {text}
    </span>
  );
}
