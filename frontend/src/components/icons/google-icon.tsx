import type { LucideIcon } from "lucide-react";
import type { SVGProps } from "react";

export function GoogleIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" {...props}>
      <path
        fill="#EA4335"
        d="M12 10.2v3.6h5.1c-.2 1.2-.9 2.3-1.9 3l3.1 2.4c1.8-1.7 2.9-4.1 2.9-7 0-.7-.1-1.3-.2-1.9H12z"
      />
      <path
        fill="#34A853"
        d="M6.6 14.3l-.8.6-2.5 1.9C5 19.4 8.2 21.4 12 21.4c2.4 0 4.4-.8 5.9-2.2l-3.1-2.4c-.8.6-1.9.9-2.8.9-2.2 0-4-1.5-4.7-3.4z"
      />
      <path
        fill="#4A90E2"
        d="M3.3 7.2C2.5 8.7 2 10.3 2 12s.5 3.3 1.3 4.8l3.3-2.5C6.2 13.4 6 12.7 6 12s.2-1.4.5-2.1L3.3 7.2z"
      />
      <path
        fill="#FBBC05"
        d="M12 5.9c1.3 0 2.5.5 3.4 1.3l2.6-2.6C16.4 2.9 14.4 2 12 2 8.2 2 5 4 3.3 7.2l3.3 2.5C7.9 7.4 9.8 5.9 12 5.9z"
      />
    </svg>
  );
}

function GoogleNavIcon({ className }: { className?: string }) {
  return <GoogleIcon className={className} />;
}

export const googleNavIcon = GoogleNavIcon as LucideIcon;
