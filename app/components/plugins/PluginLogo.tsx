import { derivePluginInitials, getPluginFallbackColor } from "./logoColor";
import { useState } from "react";

import { cn } from "@/libraries/utils";

export interface PluginLogoProps {
  scope: string;
  id: string;
  imageUrl?: string | null;
  size: 24 | 40 | 56 | 64;
  className?: string;
}

const RADIUS_BY_SIZE: Record<PluginLogoProps["size"], number> = {
  24: 4,
  40: 6,
  56: 10,
  64: 12,
};

const PADDING_BY_SIZE: Record<PluginLogoProps["size"], number> = {
  24: 3,
  40: 6,
  56: 8,
  64: 10,
};

export const PluginLogo = ({
  scope,
  id,
  imageUrl,
  size,
  className,
}: PluginLogoProps) => {
  const [hasError, setHasError] = useState(false);
  const showImage = !!imageUrl && !hasError;
  const radius = RADIUS_BY_SIZE[size];
  const padding = PADDING_BY_SIZE[size];

  return (
    <div
      className={cn(
        "inline-flex shrink-0 items-center justify-center overflow-hidden",
        className,
      )}
      style={{
        width: size,
        height: size,
        aspectRatio: "1 / 1",
        borderRadius: radius,
        backgroundColor: showImage
          ? "oklch(1 0 0 / 5%)"
          : getPluginFallbackColor(scope, id),
      }}
    >
      {showImage ? (
        <img
          alt={`${id} logo`}
          onError={() => setHasError(true)}
          src={imageUrl!}
          style={{
            width: "100%",
            height: "100%",
            padding,
            objectFit: "contain",
          }}
        />
      ) : (
        <span
          className="font-semibold text-white"
          style={{ fontSize: size * 0.4 }}
        >
          {derivePluginInitials(id)}
        </span>
      )}
    </div>
  );
};
