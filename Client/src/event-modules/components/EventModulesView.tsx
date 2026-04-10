import React from 'react';
import { Card, CardContent } from '../../components/ui';

const EventModulesView: React.FC = () => {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text-main mb-1">Event Modules</h1>
          <p className="text-sm text-text-muted">Manage and interact with game events.</p>
        </div>
      </div>

      <Card>
        <CardContent className="flex items-center justify-center py-20">
          <p className="text-text-muted font-medium">Event modules features coming soon...</p>
        </CardContent>
      </Card>
    </div>
  );
};

export default EventModulesView;
