import React from 'react';

interface LevelBadgeProps {
  level: number;
  imageSize: number;
}

const LevelBadge: React.FC<LevelBadgeProps> = ({ level, imageSize }) => {
  const badgeSize = Math.max(18, imageSize * 0.32);

  return (
    <span
      className="pointer-events-none absolute left-0 top-0 z-20 flex items-center justify-center"
      style={{
        width: badgeSize,
        height: badgeSize,
        transform: 'translate(-10%, -10%)',
      }}
      aria-label={`Level ${level}`}
    >
      <svg viewBox="0 0 100 100" className="absolute inset-0 h-full w-full drop-shadow-lg" aria-hidden="true">
        <polygon
          points="50,2 95,25 95,75 50,98 5,75 5,25"
          fill="url(#levelGradient)"
          stroke="rgba(255,255,255,0.4)"
          strokeWidth="4"
        />
      </svg>
      <span
        className="relative z-10 font-bold text-white"
        style={{
          fontSize: Math.max(9, imageSize * 0.16),
          textShadow: '0 1px 2px rgba(0,0,0,0.5)',
        }}
      >
        {level}
      </span>
    </span>
  );
};

export default LevelBadge;
