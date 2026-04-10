import React from 'react';
import { useAuth } from '../context/AuthContext';
import { Button, type ButtonProps } from './ui';

const GameButton: React.FC<ButtonProps> = ({ children, className, disabled, ...props }) => {
  const { gameLoggedIn } = useAuth();

  const isDisabled = disabled || !gameLoggedIn;

  const buttonContent = !gameLoggedIn ? (
    <span className="flex items-center gap-2">
      <span>Start Bot to {typeof children === 'string' ? 'Enable' : 'Use'}</span>
    </span>
  ) : (
    children
  );

  return (
    <Button
      disabled={isDisabled}
      className={`${className || ''} ${!gameLoggedIn ? 'cursor-not-allowed opacity-50 grayscale' : ''}`.trim()}
      {...props}
    >
      {buttonContent}
    </Button>
  );
};

export default GameButton;
