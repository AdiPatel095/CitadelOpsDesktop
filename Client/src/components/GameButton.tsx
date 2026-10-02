import React from 'react';
import { useLocale } from '../i18n/LocaleContext';
import { useAuth } from '../context/AuthContext';
import { Button, type ButtonProps } from './ui';

type GameButtonProps = ButtonProps & { loggedOutAction?: 'enable' | 'use' };
const GameButton: React.FC<GameButtonProps> = ({ children, className, disabled, loggedOutAction = 'use', ...props }) => {
  const { message } = useLocale();
  const loggedOutMessage = message(loggedOutAction === 'enable' ? 'gameButton.startToEnable' : 'gameButton.startToUse');
  const { gameLoggedIn } = useAuth();

  const isDisabled = disabled || !gameLoggedIn;

  const buttonContent = !gameLoggedIn ? (
    <span className="flex items-center gap-2">
      <span lang={loggedOutMessage.resolvedLocale}>{loggedOutMessage.text}</span>
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
