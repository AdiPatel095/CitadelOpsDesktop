import React from 'react';

const EventModulesView: React.FC = () => {
    return (
        <div className="flex flex-col gap-6">
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-2xl font-bold text-text-main mb-1">Event Modules</h1>
                    <p className="text-sm text-text-muted">Manage and interact with game events.</p>
                </div>
            </div>

            <div className="bg-bg-card border border-border-base rounded-global p-6 shadow-sm">
                <div className="text-text-muted text-center py-10">
                    <p>Event modules features coming soon...</p>
                </div>
            </div>
        </div>
    );
};

export default EventModulesView;
