import React, { useState } from 'react';
import { Shield } from 'lucide-react';
import { getUnitBaseAndLevel } from '../config/Constants';
import { useMetadata } from '../context/MetadataContext';
import LevelBadge from './LevelBadge';

interface UnitImageProps {
  unitId: number;
  size?: number;
  showLevel?: boolean;
  className?: string;
}

const UnitImage: React.FC<UnitImageProps> = ({ unitId, size = 40, showLevel = false, className = '' }) => {
  const { getTroop } = useMetadata();
  const [failed, setFailed] = useState(false);
  const metadata = getTroop(unitId);
  const level = getUnitBaseAndLevel(unitId)?.level;
  const src = metadata?.image || `/assets/Troops/${unitId}.png`;
  const title = metadata?.name ?? `Unit ${unitId}`;

  return (
    <span
      className={`relative inline-flex shrink-0 items-center justify-center rounded-global bg-bg-app ${className}`}
      style={{ width: size, height: size }}
      title={title}
      aria-label={title}
    >
      {!failed ? (
        <img
          src={src}
          alt={title}
          width={size}
          height={size}
          className="h-full w-full object-contain"
          draggable={false}
          onError={() => setFailed(true)}
        />
      ) : (
        <Shield className="h-1/2 w-1/2 text-text-muted" />
      )}
      {showLevel && level && <LevelBadge level={level} imageSize={size} />}
    </span>
  );
};

export default UnitImage;
