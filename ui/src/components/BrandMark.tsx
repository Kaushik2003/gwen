import logo from "../assets/brand/logo.png";

/** Gwen's mark: the app logo. */
export default function BrandMark({ className }: { className?: string }) {
  return <img src={logo} alt="" aria-hidden className={`rounded-[25%] object-cover ${className ?? ""}`} />;
}
