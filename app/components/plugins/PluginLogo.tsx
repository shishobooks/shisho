import { useState } from "react";

import { cn } from "@/libraries/utils";

import { derivePluginInitials, getPluginFallbackColor } from "./logoColor";

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

  return (
    <div
      aria-label={!showImage ? `${id} logo` : undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center overflow-hidden",
        className,
      )}
      role={!showImage ? "img" : undefined}
      style={{
        width: size,
        height: size,
        aspectRatio: "1 / 1",
        borderRadius: radius,
        backgroundColor: showImage
          ? undefined
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
            objectFit: "contain",
          }}
        />
      ) : (
        <span
          aria-hidden="true"
          className="font-semibold text-white"
          style={{ fontSize: size * 0.4 }}
        >
          {derivePluginInitials(id)}
        </span>
      )}
    </div>
  );
};
