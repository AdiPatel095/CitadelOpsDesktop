import React, { useEffect, useMemo, useState } from 'react';
import { useMetadata } from '../context/MetadataContext';

interface ToolImageProps {
    toolId: number;
    size?: number;
    className?: string;
}

const ToolImage: React.FC<ToolImageProps> = ({
    toolId,
    size = 64,
    className = ''
}) => {
    const [imageFailed, setImageFailed] = useState(false);
    const { getToolImageUrl, getTool } = useMetadata();

    const imageSrc = useMemo(() => getToolImageUrl(toolId), [toolId, getToolImageUrl]);
    const toolInfo = getTool(toolId);
    const altText = toolInfo?.name || `Tool ${toolId}`;

    useEffect(() => {
        setImageFailed(false);
    }, [imageSrc]);

    return (
        <div
            className={`tool-image-container relative inline-block ${className}`}
            style={{ width: size, height: size }}
        >
            {imageFailed || !imageSrc ? (
                <div
                    className="w-full h-full object-contain rounded-lg flex items-center justify-center bg-bg-card border border-border-base"
                    style={{ width: size, height: size }}
                    title={`Missing asset for tool ${toolId}`}
                >
                    <span
                        className="font-semibold text-text-muted tabular-nums text-center px-1 break-words"
                        style={{ fontSize: Math.max(10, size * 0.22) }}
                    >
                        {toolInfo?.name || toolId}
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
        </div>
    );
};

export default ToolImage;
