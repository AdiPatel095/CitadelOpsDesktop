import React from 'react';
import StaleSessionBanner from '../components/StaleSessionBanner';
import EventScoreCard from '../dashboard/components/EventScoreCard';
import { useAuth } from '../context/AuthContext';
import { PageHeader } from '../components/ui';

const EventsView: React.FC = () => {
  const { gameLoggedIn } = useAuth();

  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />
      <PageHeader title="Events" description="Review your active event score and progress." />
      <EventScoreCard live={gameLoggedIn} />
    </div>
  );
};

export default EventsView;
