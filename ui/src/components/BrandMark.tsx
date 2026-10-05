/** Gwen's mark: a G drawn as a clock ring with its hand. */
export default function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden>
      <rect width="24" height="24" rx="6" fill="#5e6ad2" />
      <path d="M16.6 7.4A6.5 6.5 0 1 0 18.5 12H12.5" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
