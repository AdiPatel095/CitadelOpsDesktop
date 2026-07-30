import React, { useEffect, useState } from 'react';
import { Blocks } from 'lucide-react';

interface TCIImageProps {
  src?: string;
  alt: string;
  size?: number;
  className?: string;
}

const TCIImage: React.FC<TCIImageProps> = ({ src = '', alt, size = 88, className = '' }) => {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [src]);

  return (
    <span
      className={`tci-image ${className}`}
      style={{ width: size, height: size }}
      aria-label={alt}
      title={alt}
    >
      {src && !failed ? (
        <img
          src={src}
          alt={alt}
          width={size}
          height={size}
          loading="lazy"
          decoding="async"
          draggable={false}
          onError={() => setFailed(true)}
        />
      ) : (
        <Blocks aria-hidden="true" />
      )}
    </span>
  );
};

export default TCIImage;
