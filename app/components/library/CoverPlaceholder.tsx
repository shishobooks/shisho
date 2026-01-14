import { cn } from "@/libraries/utils";

interface CoverPlaceholderProps {
  variant: "book" | "audiobook";
  className?: string;
}

// Book icon - larger and centered for better visibility at small sizes
const BookIcon = () => (
  <g
    fill="none"
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth="6"
    transform="translate(50, 90)"
  >
    <path d="M0 0 L0 120 L100 120 L100 10 L90 0 Z" />
    <path d="M0 0 L90 0" />
    <path d="M90 0 L90 10 L100 10" />
    <path d="M10 0 L10 120" />
  </g>
);

// Headphones icon - centered in 300x300 viewBox
const HeadphonesIcon = () => (
  <g
    fill="none"
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth="6"
    transform="translate(90, 95)"
  >
    <path d="M0 70 L0 50 C0 20 27 0 60 0 C93 0 120 20 120 50 L120 70" />
    <rect height="40" rx="4" width="24" x="0" y="70" />
    <rect height="40" rx="4" width="24" x="96" y="70" />
  </g>
);

function CoverPlaceholder({ variant, className }: CoverPlaceholderProps) {
  const isBook = variant === "book";
  const viewBox = isBook ? "0 0 200 300" : "0 0 300 300";

  return (
    <div className={cn("relative w-full", className)}>
      {/* Light mode SVG */}
      <svg
        className="absolute inset-0 w-full h-full dark:hidden"
        preserveAspectRatio="xMidYMid slice"
        viewBox={viewBox}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id="bg-light" x1="0%" x2="0%" y1="0%" y2="100%">
            <stop offset="0%" stopColor="#f0eef5" />
            <stop offset="100%" stopColor="#e0dcec" />
          </linearGradient>
        </defs>
        <rect fill="url(#bg-light)" height="100%" width="100%" />
        <g stroke="#9b8fb5">{isBook ? <BookIcon /> : <HeadphonesIcon />}</g>
      </svg>

      {/* Dark mode SVG */}
      <svg
        className="absolute inset-0 w-full h-full hidden dark:block"
        preserveAspectRatio="xMidYMid slice"
        viewBox={viewBox}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id="bg-dark" x1="0%" x2="0%" y1="0%" y2="100%">
            <stop offset="0%" stopColor="#3a3545" />
            <stop offset="100%" stopColor="#2d2a33" />
          </linearGradient>
        </defs>
        <rect fill="url(#bg-dark)" height="100%" width="100%" />
        <g stroke="#7d7399">{isBook ? <BookIcon /> : <HeadphonesIcon />}</g>
      </svg>
    </div>
  );
}

export default CoverPlaceholder;
