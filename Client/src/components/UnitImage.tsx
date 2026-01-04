import React from 'react';
import { getUnitBaseAndLevel } from '../config/constants';

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
 * - If found: renders base image with hexagonal level badge
 * - If not found: renders the unitId image directly (no level badge)
 */
const UnitImage: React.FC<UnitImageProps> = ({
    unitId,
    size = 64,
    showLevel = true,
    className = ''
}) => {
    // Check if this unit has a level mapping
    const levelInfo = getUnitBaseAndLevel(unitId);

    // Determine which image to load
    const imageId = levelInfo ? levelInfo.baseId : unitId;
    const level = levelInfo?.level;

    // Path to the troop image
    const imageSrc = `/assets/Troops/${imageId}.png`;

    return (
        <div
            className={`unit-image-container relative inline-block ${className}`}
            style={{ width: size, height: size }}
        >
            {/* Unit Image */}
            <img
                src={imageSrc}
                alt={`Unit ${unitId}`}
                className="w-full h-full object-contain rounded-lg"
                style={{ width: size, height: size }}
                onError={(e) => {
                    // Fallback to placeholder if image not found
                    (e.target as HTMLImageElement).src = '/assets/Troops/6.png';
                }}
            />

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
                        <defs>
                            <linearGradient id="levelGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                                <stop offset="0%" stopColor="#3b82f6" />
                                <stop offset="100%" stopColor="#1d4ed8" />
                            </linearGradient>
                        </defs>
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
