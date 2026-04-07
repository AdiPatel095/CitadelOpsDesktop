import React from 'react';

/**
 * SharedSvgDefs
 *
 * Provides shared SVG gradient definitions used across the app.
 * Rendered once at app root to avoid duplicating defs in every UnitImage instance.
 */
const SharedSvgDefs: React.FC = () => {
    return (
        <svg width="0" height="0" style={{ position: 'absolute' }}>
            <defs>
                <linearGradient id="levelGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                    <stop offset="0%" stopColor="#3b82f6" />
                    <stop offset="100%" stopColor="#1d4ed8" />
                </linearGradient>
            </defs>
        </svg>
    );
};

export default SharedSvgDefs;
