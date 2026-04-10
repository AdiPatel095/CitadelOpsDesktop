import React, { useEffect, useMemo, useState } from 'react';
import { getUnitBaseAndLevel } from '../config/constants';
import { useMetadata } from '../context/MetadataContext';

interface UnitImageProps {
    unitId: number;
    size?: number;
    showLevel?: boolean;
    className?: string;
}

/**
 * UnitImage Component
 *
 * Renders a unit image with optional level badge overlay.
 * - Looks up unitId in UNIT_TO_BASE_MAP to find base image + level
 * - Fetches the image path from the metadata context
 */
const UnitImage: React.FC<UnitImageProps> = ({
    unitId,
    size = 64,
    showLevel = true,
    className = ''
}) => {
    const [imageFailed, setImageFailed] = useState(false);
    const { getTroopImageUrl, getTroop } = useMetadata();

    // Check if this unit has a level mapping
    const levelInfo = getUnitBaseAndLevel(unitId);

    // Determine which image to load
    const imageId = levelInfo ? levelInfo.baseId : unitId;
    const level = levelInfo?.level;

    const imageSrc = useMemo(() => getTroopImageUrl(imageId), [imageId, getTroopImageUrl]);
    const troopInfo = getTroop(unitId);
    const altText = troopInfo?.name || `Unit ${unitId}`;

    useEffect(() => {
        setImageFailed(false);
    }, [imageSrc]);

    return (
        <div
            className={`unit-image-container relative inline-block ${className}`}
            style={{ width: size, height: size }}
        >
            {/* Unit Image — avoid chaining to another missing PNG on error */}
            {imageFailed || !imageSrc ? (
                <div
                    className="w-full h-full object-contain rounded-lg flex items-center justify-center bg-bg-card border border-border-base"
                    style={{ width: size, height: size }}
                    title={`Missing asset for unit ${imageId}`}
                >
                    <span
                        className="font-semibold text-text-muted tabular-nums text-center px-1 break-words"
                        style={{ fontSize: Math.max(10, size * 0.22) }}
                    >
                        {troopInfo?.name || unitId}
                    </span>
                </div>
            ) : (
                <img
                    src={imageSrc}
                    alt={altText}
                    className="w-full h-full object-contain rounded-lg"
                    style={{ width: size, height: size }}
                    loading="lazy"
                    decoding="async"
                    onError={() => setImageFailed(true)}
                />
            )}

            {/* Level Badge - Hexagonal style (smaller) */}
            {showLevel && level && (
                <div
                    className="absolute top-0 left-0 flex items-center justify-center"
                    style={{
                        width: size * 0.28,
                        height: size * 0.28,
                        transform: 'translate(-10%, -10%)',
                    }}
                >
                    {/* Hexagon background */}
                    <svg
                        viewBox="0 0 100 100"
                        className="absolute inset-0 w-full h-full drop-shadow-lg"
                    >
                        <polygon
                            points="50,2 95,25 95,75 50,98 5,75 5,25"
                            fill="url(#levelGradient)"
                            stroke="rgba(255,255,255,0.4)"
                            strokeWidth="4"
                        />
                    </svg>
                    {/* Level number */}
                    <span
                        className="relative z-10 font-bold text-white"
                        style={{
                            fontSize: size * 0.16,
                            textShadow: '0 1px 2px rgba(0,0,0,0.5)'
                        }}
                    >
                        {level}
                    </span>
                </div>
            )}
        </div>
    );
};

export default UnitImage;
