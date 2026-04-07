import React, { useEffect, useMemo, useState } from 'react';
import { getUnitBaseAndLevel } from '../config/constants';

interface UnitImageProps {
    unitId: number;
    size?: number;
    showLevel?: boolean;
    className?: string;
}

function troopPngSrc(imageId: number): string {
    const base = import.meta.env.BASE_URL;
    const prefix = base.endsWith('/') ? base.slice(0, -1) : base;
    return `${prefix}/assets/Troops/${imageId}.png`;
}

/**
 * UnitImage Component
 *
 * Renders a unit image with optional level badge overlay.
 * - Looks up unitId in UNIT_TO_BASE_MAP to find base image + level
 * - If found: renders base image with hexagonal level badge
 * - If not found: renders the unitId image directly (no level badge)
 *
 * Place PNGs at Client/public/assets/Troops/{id}.png (id = base unit for leveled troops)
 * so they are copied into dist and served by the embedded dashboard.
 */
const UnitImage: React.FC<UnitImageProps> = ({
    unitId,
    size = 64,
    showLevel = true,
    className = ''
}) => {
    const [imageFailed, setImageFailed] = useState(false);

    // Check if this unit has a level mapping
    const levelInfo = getUnitBaseAndLevel(unitId);

    // Determine which image to load
    const imageId = levelInfo ? levelInfo.baseId : unitId;
    const level = levelInfo?.level;

    const imageSrc = useMemo(() => troopPngSrc(imageId), [imageId]);

    useEffect(() => {
        setImageFailed(false);
    }, [imageSrc]);

    return (
        <div
            className={`unit-image-container relative inline-block ${className}`}
            style={{ width: size, height: size }}
        >
            {/* Unit Image — avoid chaining to another missing PNG on error */}
            {imageFailed ? (
                <div
                    className="w-full h-full object-contain rounded-lg flex items-center justify-center bg-bg-card border border-border-base"
                    style={{ width: size, height: size }}
                    title={`Missing asset: assets/Troops/${imageId}.png`}
                >
                    <span
                        className="font-semibold text-text-muted tabular-nums"
                        style={{ fontSize: Math.max(10, size * 0.22) }}
                    >
                        {unitId}
                    </span>
                </div>
            ) : (
                <img
                    src={imageSrc}
                    alt={`Unit ${unitId}`}
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
